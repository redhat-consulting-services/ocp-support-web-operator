package controller

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"

	supportv1alpha1 "github.com/redhat-consulting-services/ocp-support-web-operator/api/v1alpha1"
)

type SupportGatherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SupportGatherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &supportv1alpha1.SupportGather{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if instance.Status.Phase == "Complete" || instance.Status.Phase == "Failed" {
		return ctrl.Result{}, nil
	}

	backendURL, err := r.resolveBackendURL(ctx, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", "", fmt.Sprintf("cannot resolve backend: %v", err))
	}

	switch instance.Status.Phase {
	case "":
		return r.handlePending(ctx, instance, backendURL)
	case "Pending":
		return r.handlePending(ctx, instance, backendURL)
	case "Gathering":
		return r.handleGathering(ctx, instance, backendURL)
	case "Uploading":
		return r.handleUploading(ctx, instance, backendURL)
	default:
		logger.Info("Unknown phase", "phase", instance.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *SupportGatherReconciler) handlePending(ctx context.Context, instance *supportv1alpha1.SupportGather, backendURL string) (ctrl.Result, error) {
	body := map[string]interface{}{}

	if len(instance.Spec.Namespaces) > 0 {
		body["types"] = []string{"custom"}
		body["namespaces"] = instance.Spec.Namespaces
		if len(instance.Spec.ResourceTypes) > 0 {
			body["resourceTypes"] = instance.Spec.ResourceTypes
		}
		if instance.Spec.IncludeLogs != nil {
			body["includeLogs"] = *instance.Spec.IncludeLogs
		}
	} else {
		types := instance.Spec.GatherTypes
		if len(types) == 0 {
			types = []string{"all"}
		}
		body["types"] = types
	}

	if instance.Spec.Anonymize {
		body["anonymize"] = true
	}
	if instance.Spec.Since != "" {
		body["since"] = instance.Spec.Since
	}

	resp, err := r.backendPost(backendURL+"/api/support/gather", body)
	if err != nil {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.setStatus(ctx, instance, "Pending", "", fmt.Sprintf("backend request failed: %v", err))
	}

	jobID, _ := resp["id"].(string)
	if jobID == "" {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", "", "backend returned no job ID")
	}

	if err := r.setStatus(ctx, instance, "Gathering", jobID, ""); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *SupportGatherReconciler) handleGathering(ctx context.Context, instance *supportv1alpha1.SupportGather, backendURL string) (ctrl.Result, error) {
	jobID := instance.Status.JobID
	if jobID == "" {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", "", "no job ID in status")
	}

	status, err := r.backendGet(backendURL + "/api/support/gather/" + jobID)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	jobStatus, _ := status["status"].(string)
	step, _ := status["step"].(float64)
	totalSteps, _ := status["totalSteps"].(float64)
	fileName, _ := status["fileName"].(string)
	jobError, _ := status["error"].(string)

	latest := &supportv1alpha1.SupportGather{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, latest); err != nil {
		return ctrl.Result{}, err
	}

	if totalSteps > 0 {
		latest.Status.Progress = int(step * 100 / totalSteps)
	}
	if fileName != "" {
		latest.Status.FileName = fileName
	}

	switch jobStatus {
	case "complete":
		if instance.Spec.Upload != nil {
			latest.Status.Phase = "Uploading"
			if err := r.Status().Update(ctx, latest); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		now := metav1.Now()
		latest.Status.Phase = "Complete"
		latest.Status.CompletionTime = &now
		setCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               "Complete",
			Status:             metav1.ConditionTrue,
			Reason:             "GatherComplete",
			Message:            "Must-gather archive is ready",
			LastTransitionTime: now,
		})
		if err := r.Status().Update(ctx, latest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case "failed":
		now := metav1.Now()
		latest.Status.Phase = "Failed"
		latest.Status.Error = jobError
		latest.Status.CompletionTime = &now
		if err := r.Status().Update(ctx, latest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	default:
		if err := r.Status().Update(ctx, latest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

func (r *SupportGatherReconciler) handleUploading(ctx context.Context, instance *supportv1alpha1.SupportGather, backendURL string) (ctrl.Result, error) {
	upload := instance.Spec.Upload
	if upload == nil {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", instance.Status.JobID, "upload spec missing")
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: upload.SecretRef.Name, Namespace: instance.Namespace}, secret); err != nil {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", instance.Status.JobID, fmt.Sprintf("upload secret not found: %v", err))
	}

	username := string(secret.Data["username"])
	password := string(secret.Data["password"])
	if username == "" || password == "" {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", instance.Status.JobID, "upload secret missing username or password")
	}

	body := map[string]interface{}{
		"caseID":       upload.CaseID,
		"internalUser": upload.InternalUser,
	}

	_, err := r.backendPost(backendURL+"/api/support/upload/"+instance.Status.JobID, body)
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, instance, "Failed", instance.Status.JobID, fmt.Sprintf("upload failed: %v", err))
	}

	now := metav1.Now()
	latest := &supportv1alpha1.SupportGather{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, latest); err != nil {
		return ctrl.Result{}, err
	}
	latest.Status.Phase = "Complete"
	latest.Status.UploadStatus = fmt.Sprintf("Uploaded to case %s", upload.CaseID)
	latest.Status.CompletionTime = &now
	setCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               "Complete",
		Status:             metav1.ConditionTrue,
		Reason:             "UploadComplete",
		Message:            fmt.Sprintf("Archive uploaded to Red Hat case %s", upload.CaseID),
		LastTransitionTime: now,
	})
	return ctrl.Result{}, r.Status().Update(ctx, latest)
}

func (r *SupportGatherReconciler) resolveBackendURL(ctx context.Context, namespace string) (string, error) {
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: appName, Namespace: namespace}, svc); err != nil {
		return "", fmt.Errorf("service %s/%s not found: %w", namespace, appName, err)
	}
	return fmt.Sprintf("http://%s.%s.svc:8080", appName, namespace), nil
}

func (r *SupportGatherReconciler) setStatus(ctx context.Context, instance *supportv1alpha1.SupportGather, phase, jobID, errMsg string) error {
	latest := &supportv1alpha1.SupportGather{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, latest); err != nil {
		return err
	}
	latest.Status.Phase = phase
	if jobID != "" {
		latest.Status.JobID = jobID
	}
	if errMsg != "" {
		latest.Status.Error = errMsg
		now := metav1.Now()
		setCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			Reason:             "Error",
			Message:            errMsg,
			LastTransitionTime: now,
		})
	}
	return r.Status().Update(ctx, latest)
}

func (r *SupportGatherReconciler) backendPost(url string, body map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SupportGatherReconciler) backendGet(url string) (map[string]interface{}, error) {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SupportGatherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&supportv1alpha1.SupportGather{}).
		Complete(r)
}
