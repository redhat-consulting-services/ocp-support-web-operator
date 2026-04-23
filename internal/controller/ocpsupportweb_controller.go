package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	supportv1alpha1 "github.com/redhat-consulting-services/ocp-support-web-operator/api/v1alpha1"
)

const (
	finalizerName = "support.openshift.io/finalizer"
	appName       = "ocp-support-web"

	consoleLinkIcon = `data:image/png;base64,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`

	defaultOAuthProxyImage = "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest"
)

type OCPSupportWebReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	RelatedImages map[string]string
}

func (r *OCPSupportWebReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &supportv1alpha1.OCPSupportWeb{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, finalizerName) {
			if err := r.cleanupClusterResources(ctx, instance); err != nil {
				logger.Error(err, "Failed to clean up cluster resources")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(instance, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(instance, finalizerName) {
		controllerutil.AddFinalizer(instance, finalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	needsUpdate := false

	// If RELATED_IMAGE_APP is set and the CR image matches the previous
	// operator's default, upgrade it to the new default. This enables
	// automatic rolling upgrades when the operator is upgraded via OLM.
	if relatedApp := r.RelatedImages["RELATED_IMAGE_APP"]; relatedApp != "" {
		if instance.Spec.Image == "" || instance.Spec.Image != relatedApp {
			// Only auto-upgrade if the current image looks like an operator-managed
			// default (same repo prefix), not a user-customized image.
			if instance.Spec.Image == "" || isSameRepo(instance.Spec.Image, relatedApp) {
				instance.Spec.Image = relatedApp
				needsUpdate = true
			}
		}
	}
	if instance.Spec.Image == "" {
		r.setPhase(ctx, instance, "Failed", "Image must be set in spec.image or RELATED_IMAGE_APP env var")
		return ctrl.Result{}, fmt.Errorf("no application image configured")
	}

	if instance.Spec.OAuthProxyImage == "" {
		instance.Spec.OAuthProxyImage = r.resolveImage("", "RELATED_IMAGE_OAUTH_PROXY", defaultOAuthProxyImage)
		needsUpdate = true
	}

	if instance.Spec.ClusterDomain == "" {
		detected, err := r.detectClusterDomain(ctx)
		if err != nil {
			logger.Error(err, "Could not auto-detect cluster domain")
		} else {
			instance.Spec.ClusterDomain = detected
			needsUpdate = true
		}
	}

	if needsUpdate {
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	appImage := instance.Spec.Image
	agentImage := instance.Spec.AgentImage
	if agentImage == "" {
		agentImage = appImage
	}
	oauthProxyImage := instance.Spec.OAuthProxyImage
	clusterDomain := instance.Spec.ClusterDomain

	ns := instance.Namespace

	if err := r.reconcileServiceAccount(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("ServiceAccount: %v", err))
	}

	if err := r.reconcileClusterRoleBinding(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("ClusterRoleBinding: %v", err))
	}

	if err := r.reconcileMonitoringRoleBinding(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("MonitoringRoleBinding: %v", err))
	}

	if err := r.reconcileViewerRBAC(ctx, instance); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("ViewerRBAC: %v", err))
	}

	if err := r.reconcileAuthConfigMap(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Auth ConfigMap: %v", err))
	}

	if err := r.reconcileGatherConfigMap(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Gather ConfigMap: %v", err))
	}

	if err := r.reconcileCookieSecret(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Cookie Secret: %v", err))
	}

	if err := r.reconcileSFTPKnownHosts(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("SFTP Known Hosts: %v", err))
	}

	if err := r.reconcileService(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Service: %v", err))
	}

	if err := r.reconcileAPIService(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("API Service: %v", err))
	}

	if err := r.reconcileAppMetrics(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Metrics: %v", err))
	}

	if err := r.reconcileDeployment(ctx, instance, ns, appImage, agentImage, oauthProxyImage, clusterDomain); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Deployment: %v", err))
	}

	routeURL, routeHost, err := r.reconcileRoute(ctx, instance, ns)
	if err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Route: %v", err))
	}

	if routeURL != "" {
		if err := r.reconcileConsoleLink(ctx, routeURL); err != nil {
			logger.Error(err, "Failed to reconcile ConsoleLink")
		}
	}

	if routeHost != "" && (instance.Spec.Route == nil || instance.Spec.Route.Host != routeHost) {
		if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
			return ctrl.Result{}, err
		}
		if instance.Spec.Route == nil {
			instance.Spec.Route = &supportv1alpha1.RouteSpec{}
		}
		instance.Spec.Route.Host = routeHost
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.Phase = "Available"
	instance.Status.RouteURL = routeURL
	instance.Status.AppImage = appImage
	now := metav1.Now()
	setCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionTrue,
		Reason:             "Deployed",
		Message:            "All resources are deployed",
		LastTransitionTime: now,
	})
	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
	}

	logger.Info("Reconciliation complete", "routeURL", routeURL)
	return ctrl.Result{}, nil
}

func (r *OCPSupportWebReconciler) resolveImage(specImage, envKey, fallback string) string {
	if specImage != "" {
		return specImage
	}
	if v := r.RelatedImages[envKey]; v != "" {
		return v
	}
	return fallback
}

func (r *OCPSupportWebReconciler) detectClusterDomain(ctx context.Context) (string, error) {
	ingress := &unstructured.Unstructured{}
	ingress.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "Ingress",
	})
	if err := r.Get(ctx, types.NamespacedName{Name: "cluster"}, ingress); err != nil {
		return "", err
	}
	domain, found, err := unstructured.NestedString(ingress.Object, "spec", "domain")
	if err != nil || !found {
		return "", fmt.Errorf("spec.domain not found in Ingress/cluster")
	}
	return domain, nil
}

func (r *OCPSupportWebReconciler) reconcileServiceAccount(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: ns,
			Labels:    labels(),
			Annotations: map[string]string{
				"serviceaccounts.openshift.io/oauth-redirectreference.proxy": `{"kind":"OAuthRedirectReference","apiVersion":"v1","reference":{"kind":"Route","name":"` + appName + `"}}`,
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, sa, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, sa)
	}
	if err != nil {
		return err
	}

	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	for k, v := range sa.Annotations {
		existing.Annotations[k] = v
	}
	existing.Labels = labels()
	return r.Update(ctx, existing)
}

func (r *OCPSupportWebReconciler) reconcileClusterRoleBinding(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	crName := appName + "-" + ns
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crName,
			Labels: labels(),
		},
		Rules: appClusterRoleRules(),
	}

	existingCR := &rbacv1.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{Name: crName}, existingCR)
	if errors.IsNotFound(err) {
		if err := r.Create(ctx, cr); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		existingCR.Rules = cr.Rules
		existingCR.Labels = labels()
		if err := r.Update(ctx, existingCR); err != nil {
			return err
		}
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crName,
			Labels: labels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     crName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      appName,
				Namespace: ns,
			},
		},
	}

	existingCRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: crName}, existingCRB)
	if errors.IsNotFound(err) {
		return r.Create(ctx, crb)
	}
	if err != nil {
		return err
	}

	existingCRB.RoleRef = crb.RoleRef
	existingCRB.Subjects = crb.Subjects
	existingCRB.Labels = labels()
	return r.Update(ctx, existingCRB)
}

func appClusterRoleRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		// Read-only access to all resources across all API groups (must-gather diagnostics)
		{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "list"}},
		// Pods — create/delete for node debug pods
		{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"create", "delete"}},
		{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
		{APIGroups: []string{""}, Resources: []string{"pods/proxy"}, Verbs: []string{"get", "create"}},
		// Privileged SCC — needed for node debug pods (hostNetwork, hostPID, hostPath)
		{APIGroups: []string{"security.openshift.io"}, Resources: []string{"securitycontextconstraints"}, ResourceNames: []string{"privileged"}, Verbs: []string{"use"}},
		// OCS/ODF — patch needed for enabling ceph toolbox
		{APIGroups: []string{"ocs.openshift.io"}, Resources: []string{"storageclusters"}, Verbs: []string{"patch"}},
		// ACM — ManifestWork lifecycle for agent deployment
		{APIGroups: []string{"work.open-cluster-management.io"}, Resources: []string{"manifestworks"}, Verbs: []string{"create", "update", "delete"}},
		// ArgoCD — delete needed for app management
		{APIGroups: []string{"argoproj.io"}, Resources: []string{"applications", "argocds"}, Verbs: []string{"delete"}},
	}
}

func (r *OCPSupportWebReconciler) reconcileMonitoringRoleBinding(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	crbName := appName + "-" + ns + "-monitoring"

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crbName,
			Labels: labels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      appName,
				Namespace: ns,
			},
		},
	}

	existing := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: crbName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, crb)
	}
	if err != nil {
		return err
	}

	existing.RoleRef = crb.RoleRef
	existing.Subjects = crb.Subjects
	existing.Labels = labels()
	return r.Update(ctx, existing)
}

func (r *OCPSupportWebReconciler) reconcileViewerRBAC(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb) error {
	crName := appName + "-viewer"

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crName,
			Labels: labels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"support.openshift.io"},
				Resources: []string{"ocpsupportwebs"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	existingCR := &rbacv1.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{Name: crName}, existingCR)
	if errors.IsNotFound(err) {
		if err := r.Create(ctx, cr); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		existingCR.Rules = cr.Rules
		existingCR.Labels = labels()
		if err := r.Update(ctx, existingCR); err != nil {
			return err
		}
	}

	allowedGroups := owner.Spec.AllowedGroups
	if len(allowedGroups) == 0 {
		allowedGroups = []string{"cluster-admins"}
	}

	crbName := appName + "-viewer"
	subjects := make([]rbacv1.Subject, len(allowedGroups))
	for i, g := range allowedGroups {
		subjects[i] = rbacv1.Subject{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Group",
			Name:     g,
		}
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crbName,
			Labels: labels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     crName,
		},
		Subjects: subjects,
	}

	existingCRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: crbName}, existingCRB)
	if errors.IsNotFound(err) {
		return r.Create(ctx, crb)
	}
	if err != nil {
		return err
	}

	existingCRB.Subjects = crb.Subjects
	existingCRB.Labels = labels()
	return r.Update(ctx, existingCRB)
}

func (r *OCPSupportWebReconciler) cleanupClusterResources(ctx context.Context, instance *supportv1alpha1.OCPSupportWeb) error {
	logger := log.FromContext(ctx)

	crName := appName + "-" + instance.Namespace
	crb := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: crName}, crb); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		if err := r.Delete(ctx, crb); err != nil {
			return err
		}
	}

	cr := &rbacv1.ClusterRole{}
	if err := r.Get(ctx, types.NamespacedName{Name: crName}, cr); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		if err := r.Delete(ctx, cr); err != nil {
			return err
		}
	}

	monCRBName := appName + "-" + instance.Namespace + "-monitoring"
	monCRB := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: monCRBName}, monCRB); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		if err := r.Delete(ctx, monCRB); err != nil {
			return err
		}
	}

	viewerCRBName := appName + "-viewer"
	viewerCRB := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: viewerCRBName}, viewerCRB); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		if err := r.Delete(ctx, viewerCRB); err != nil {
			return err
		}
	}

	viewerCR := &rbacv1.ClusterRole{}
	if err := r.Get(ctx, types.NamespacedName{Name: viewerCRBName}, viewerCR); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		if err := r.Delete(ctx, viewerCR); err != nil {
			return err
		}
	}

	consoleLinkGVK := schema.GroupVersionKind{Group: "console.openshift.io", Version: "v1", Kind: "ConsoleLink"}
	cl := &unstructured.Unstructured{}
	cl.SetGroupVersionKind(consoleLinkGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: appName}, cl); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to get ConsoleLink for cleanup")
		}
	} else {
		if err := r.Delete(ctx, cl); err != nil {
			logger.Error(err, "Failed to delete ConsoleLink")
		}
	}

	return nil
}

func (r *OCPSupportWebReconciler) reconcileAuthConfigMap(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	cmName := appName + "-auth"

	// Determine allowed groups — default to cluster-admins if not specified
	allowedGroups := owner.Spec.AllowedGroups
	if len(allowedGroups) == 0 {
		allowedGroups = []string{"cluster-admins"}
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
			Labels:    labels(),
		},
		Data: map[string]string{
			"allowed-groups": strings.Join(allowedGroups, "\n"),
		},
	}
	if err := controllerutil.SetControllerReference(owner, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if existing.Data["allowed-groups"] != desired.Data["allowed-groups"] {
		existing.Data = desired.Data
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *OCPSupportWebReconciler) reconcileGatherConfigMap(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	cmName := "gather-common"

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: ns}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
			Labels:    labels(),
			Annotations: map[string]string{
				"support.openshift.io/user-managed": "true",
			},
		},
		Data: map[string]string{
			"spec": `displayName: "Custom cluster diagnostics"
# Add custom resources, commands, and log specs below.
# These are merged with the built-in defaults — you only need to
# list what you want to ADD, not the full set.
#
# clusterResources:
#   - group: "example.io"
#     version: "v1"
#     resource: "myresources"
#
# namespacedResources:
#   - group: ""
#     version: "v1"
#     resource: "configmaps"
#     namespaces: ["my-namespace"]
#
# podLogs:
#   - namespaces: ["my-namespace"]
#     maxLines: 5000
#
# podExecs:
#   - name: "my-command"
#     namespace: "my-namespace"
#     podSelector: "app=myapp"
#     container: "main"
#     command: ["cat", "/etc/config"]
#     outputFile: "my-config.txt"
#
# nodeCommands:
#   - name: "lspci"
#     command: ["chroot", "/host", "lspci", "-vv"]
#     outputFile: "lspci.txt"
`,
		},
	}
	if err := controllerutil.SetControllerReference(owner, cm, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, cm)
}

func (r *OCPSupportWebReconciler) reconcileCookieSecret(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	secretName := appName + "-cookie"
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ns}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	cookieVal := base64.StdEncoding.EncodeToString(b)[:32]

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
			Labels:    labels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"cookie-secret": []byte(cookieVal),
		},
	}
	if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, secret)
}

func (r *OCPSupportWebReconciler) reconcileSFTPKnownHosts(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	secretName := appName + "-sftp-known-hosts"
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ns}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
			Labels:    labels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"known-hosts": []byte("sftp.access.redhat.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCFQ3l2YVJ0r4MNzAZmTV2kg7rPi4WPeJNcNubvOVA4WwBV6cRsYFkIqtB1unBzTXoZHd7+adtZTgUrJ2BExImyLUQaLBu3KKo4CgGeiZMo8dfDvE2tIe/GwGtyho57TtwJVVUCljvFBvbz8+D6VunsQ6kNU53t8qCaBQNm61twTkdAHP9IESJbC7wWJqjmhmOMTav1OKQDtLEsSDc4I+s+h41LvUfw1lA7RSl9eR13TK9ySpN/uW5nBq7nUNWW5OBc3UbvpdQpDXvdUDbW0rQ2EEWvLkKubhk+RSeY/lH8peOeHYQ5ARPYfFDpo5KsKDDdKa9DfnK8N8APgtzM0r+l\n"),
		},
	}
	if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, secret)
}

func (r *OCPSupportWebReconciler) reconcileService(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: ns,
			Labels:    labels(),
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": appName + "-tls",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "proxy",
					Port:       8443,
					TargetPort: intstr.FromInt(8443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, svc)
	}
	if err != nil {
		return err
	}

	existing.Spec.Ports = svc.Spec.Ports
	existing.Spec.Selector = svc.Spec.Selector
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	for k, v := range svc.Annotations {
		existing.Annotations[k] = v
	}
	return r.Update(ctx, existing)
}

func (r *OCPSupportWebReconciler) reconcileAPIService(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	apiSvcName := appName + "-api"
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiSvcName,
			Namespace: ns,
			Labels:    labels(),
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": appName + "-api-tls",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       9443,
					TargetPort: intstr.FromInt(9443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: apiSvcName, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, svc)
	}
	if err != nil {
		return err
	}
	existing.Spec.Ports = svc.Spec.Ports
	existing.Spec.Selector = svc.Spec.Selector
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	for k, v := range svc.Annotations {
		existing.Annotations[k] = v
	}
	return r.Update(ctx, existing)
}

func (r *OCPSupportWebReconciler) reconcileAppMetrics(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) error {
	metricsServiceName := appName + "-metrics"

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      metricsServiceName,
			Namespace: ns,
			Labels:    labels(),
		},
		Spec: corev1.ServiceSpec{
			Selector: labels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       8081,
					TargetPort: intstr.FromInt(8081),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
		return err
	}

	existingSvc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: metricsServiceName, Namespace: ns}, existingSvc)
	if errors.IsNotFound(err) {
		if err := r.Create(ctx, svc); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		existingSvc.Spec.Ports = svc.Spec.Ports
		existingSvc.Spec.Selector = svc.Spec.Selector
		if err := r.Update(ctx, existingSvc); err != nil {
			return err
		}
	}

	smGVK := schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor"}
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(smGVK)
	sm.SetName(appName)
	sm.SetNamespace(ns)
	sm.SetLabels(labels())
	sm.Object["spec"] = map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": appName,
			},
		},
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "metrics",
				"interval": "30s",
				"path":     "/metrics",
			},
		},
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         owner.APIVersion,
		Kind:               owner.Kind,
		Name:               owner.Name,
		UID:                owner.UID,
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}
	sm.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	existingSM := &unstructured.Unstructured{}
	existingSM.SetGroupVersionKind(smGVK)
	err = r.Get(ctx, types.NamespacedName{Name: appName, Namespace: ns}, existingSM)
	if errors.IsNotFound(err) {
		return r.Create(ctx, sm)
	}
	if err != nil {
		return err
	}

	existingSM.Object["spec"] = sm.Object["spec"]
	return r.Update(ctx, existingSM)
}

func (r *OCPSupportWebReconciler) reconcileDeployment(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns, appImage, agentImage, oauthProxyImage, clusterDomain string) error {
	replicas := int32(1)

	appResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
	if owner.Spec.Resources != nil {
		appResources = *owner.Spec.Resources
	}

	allowedGroups := owner.Spec.AllowedGroups
	if len(allowedGroups) == 0 {
		allowedGroups = []string{"cluster-admins"}
	}

	appEnv := []corev1.EnvVar{
		{Name: "CLUSTER_DOMAIN", Value: clusterDomain},
		{Name: "AGENT_IMAGE", Value: agentImage},
		{Name: "TLS_LISTEN_ADDR", Value: "0.0.0.0:9443"},
		{Name: "TLS_CERT_FILE", Value: "/var/serving-cert/api/tls.crt"},
		{Name: "TLS_KEY_FILE", Value: "/var/serving-cert/api/tls.key"},
		{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
		}},
	}

	var containers []corev1.Container
	var volumes []corev1.Volume
	appPorts := []corev1.ContainerPort{
		{Name: "https", ContainerPort: 9443},
		{Name: "metrics", ContainerPort: 8081},
	}

	if owner.Spec.ConsolePlugin {
		appEnv = append(appEnv,
			corev1.EnvVar{Name: "CONSOLE_PLUGIN", Value: "true"},
		)
		if len(owner.Spec.AllowedGroups) > 0 {
			appEnv = append(appEnv,
				corev1.EnvVar{Name: "ALLOWED_GROUPS", Value: strings.Join(owner.Spec.AllowedGroups, ",")},
			)
		}
	} else {
		appEnv = append(appEnv, corev1.EnvVar{Name: "LISTEN_ADDR", Value: "0.0.0.0:8080"})
		appPorts = append([]corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}, appPorts...)

		proxyResources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		}
		if owner.Spec.OAuthProxyResources != nil {
			proxyResources = *owner.Spec.OAuthProxyResources
		}

		containers = append(containers, corev1.Container{
			Name:            "oauth-proxy",
			Image:           oauthProxyImage,
			ImagePullPolicy: corev1.PullAlways,
			Args:            r.oauthProxyArgs(owner),
			Ports: []corev1.ContainerPort{
				{Name: "proxy", ContainerPort: 8443},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "tls", MountPath: "/etc/tls/private", ReadOnly: true},
				{Name: "cookie-secret", MountPath: "/etc/oauth/cookie-secret", ReadOnly: true},
			},
			Resources: proxyResources,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: boolPtr(false),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			},
		})
		volumes = append(volumes,
			corev1.Volume{
				Name:         "tls",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: appName + "-tls"}},
			},
			corev1.Volume{
				Name:         "cookie-secret",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: appName + "-cookie"}},
			},
		)
	}

	containers = append(containers, corev1.Container{
		Name:            appName,
		Image:           appImage,
		ImagePullPolicy: corev1.PullAlways,
		Env:             appEnv,
		Ports:           appPorts,
		VolumeMounts: []corev1.VolumeMount{
			{Name: "api-tls", MountPath: "/var/serving-cert/api", ReadOnly: true},
		},
		Resources: appResources,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	})
	volumes = append(volumes, corev1.Volume{
		Name:         "api-tls",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: appName + "-api-tls"}},
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: ns,
			Labels:    labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: appName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: containers,
					Volumes:    volumes,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(owner, dep, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, dep)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Containers, dep.Spec.Template.Spec.Containers) ||
		!equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Volumes, dep.Spec.Template.Spec.Volumes) {
		existing.Spec.Template = dep.Spec.Template
		existing.Spec.Replicas = dep.Spec.Replicas
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *OCPSupportWebReconciler) reconcileRoute(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns string) (string, string, error) {
	routeGVK := schema.GroupVersionKind{Group: "route.openshift.io", Version: "v1", Kind: "Route"}

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(routeGVK)
	route.SetName(appName)
	route.SetNamespace(ns)
	route.SetLabels(labels())

	spec := map[string]interface{}{
		"to": map[string]interface{}{
			"kind": "Service",
			"name": appName,
		},
		"port": map[string]interface{}{
			"targetPort": "proxy",
		},
		"tls": map[string]interface{}{
			"termination": "Reencrypt",
		},
	}
	if owner.Spec.Route != nil && owner.Spec.Route.Host != "" {
		spec["host"] = owner.Spec.Route.Host
	}
	route.Object["spec"] = spec

	ownerRef := metav1.OwnerReference{
		APIVersion:         owner.APIVersion,
		Kind:               owner.Kind,
		Name:               owner.Name,
		UID:                owner.UID,
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}
	route.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(routeGVK)
	err := r.Get(ctx, types.NamespacedName{Name: appName, Namespace: ns}, existing)
	if errors.IsNotFound(err) {
		if err := r.Create(ctx, route); err != nil {
			return "", "", err
		}
		if err := r.Get(ctx, types.NamespacedName{Name: appName, Namespace: ns}, existing); err != nil {
			return "", "", nil
		}
		host, _, _ := unstructured.NestedString(existing.Object, "spec", "host")
		return "https://" + host, host, nil
	}
	if err != nil {
		return "", "", err
	}

	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	if existingSpec != nil {
		existingSpec["to"] = spec["to"]
		existingSpec["port"] = spec["port"]
		existingSpec["tls"] = spec["tls"]
		if owner.Spec.Route != nil && owner.Spec.Route.Host != "" {
			existingSpec["host"] = owner.Spec.Route.Host
		}
		existing.Object["spec"] = existingSpec
		if err := r.Update(ctx, existing); err != nil {
			return "", "", err
		}
	}

	host, _, _ := unstructured.NestedString(existing.Object, "spec", "host")
	return "https://" + host, host, nil
}

func (r *OCPSupportWebReconciler) reconcileConsoleLink(ctx context.Context, routeURL string) error {
	consoleLinkGVK := schema.GroupVersionKind{Group: "console.openshift.io", Version: "v1", Kind: "ConsoleLink"}
	linkName := appName

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(consoleLinkGVK)
	desired.SetName(linkName)
	desired.Object["spec"] = map[string]interface{}{
		"href":     routeURL,
		"location": "ApplicationMenu",
		"text":     "Support Web",
		"applicationMenu": map[string]interface{}{
			"section":  "Support Web",
			"imageURL": consoleLinkIcon,
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(consoleLinkGVK)
	err := r.Get(ctx, types.NamespacedName{Name: linkName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existingHref, _, _ := unstructured.NestedString(existing.Object, "spec", "href")
	if existingHref != routeURL {
		existing.Object["spec"] = desired.Object["spec"]
		return r.Update(ctx, existing)
	}
	return nil
}

// isSameRepo returns true if both images share the same repository
// (everything before the last colon/tag). This lets the operator auto-upgrade
// its own default images without overwriting user-customized images.
func isSameRepo(a, b string) bool {
	return imageRepo(a) == imageRepo(b)
}

func imageRepo(img string) string {
	// Strip tag or digest
	if at := strings.LastIndex(img, "@"); at != -1 {
		return img[:at]
	}
	if colon := strings.LastIndex(img, ":"); colon != -1 {
		// Make sure the colon is after the last slash (it's a tag, not a port)
		if slash := strings.LastIndex(img, "/"); colon > slash {
			return img[:colon]
		}
	}
	return img
}

func labels() map[string]string {
	return map[string]string{
		"app":                          appName,
		"app.kubernetes.io/name":       appName,
		"app.kubernetes.io/managed-by": "ocp-support-web-operator",
	}
}

func boolPtr(b bool) *bool { return &b }

func (r *OCPSupportWebReconciler) setPhase(ctx context.Context, instance *supportv1alpha1.OCPSupportWeb, phase, message string) error {
	latest := &supportv1alpha1.OCPSupportWeb{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, latest); err != nil {
		return err
	}
	instance = latest
	instance.Status.Phase = phase
	now := metav1.Now()
	condStatus := metav1.ConditionTrue
	reason := "Deployed"
	if phase == "Failed" {
		condStatus = metav1.ConditionFalse
		reason = "Error"
	}
	setCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             condStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
	return r.Status().Update(ctx, instance)
}

func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == cond.Type {
			if c.Status != cond.Status {
				(*conditions)[i] = cond
			} else {
				(*conditions)[i].Reason = cond.Reason
				(*conditions)[i].Message = cond.Message
			}
			return
		}
	}
	*conditions = append(*conditions, cond)
}

func (r *OCPSupportWebReconciler) oauthProxyArgs(owner *supportv1alpha1.OCPSupportWeb) []string {
	args := []string{
		"--https-address=:8443",
		"--provider=openshift",
		"--openshift-service-account=" + appName,
		"--upstream=http://localhost:8080",
		"--tls-cert=/etc/tls/private/tls.crt",
		"--tls-key=/etc/tls/private/tls.key",
		"--cookie-secret-file=/etc/oauth/cookie-secret/cookie-secret",
		"--cookie-expire=8h0m0s",
		"--cookie-refresh=1h0m0s",
		"--pass-user-headers=true",
	}

	allowedGroups := owner.Spec.AllowedGroups
	if len(allowedGroups) == 0 {
		allowedGroups = []string{"cluster-admins"}
	}
	for _, group := range allowedGroups {
		args = append(args, "--openshift-group="+group)
	}

	return args
}

func (r *OCPSupportWebReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&supportv1alpha1.OCPSupportWeb{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
