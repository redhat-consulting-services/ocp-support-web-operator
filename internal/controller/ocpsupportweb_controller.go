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

	defaultOAuthProxyImage          = "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest"
	defaultStandardMustGather       = "registry.redhat.io/openshift4/ose-must-gather-rhel9:latest"
	defaultCNVMustGather            = "registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0"
	defaultODFMustGather            = "registry.redhat.io/odf4/ocs-must-gather-rhel9:latest"
	defaultLoggingMustGather    = "registry.redhat.io/openshift-logging/cluster-logging-must-gather-rhel9:latest"
	defaultComplianceMustGather = "registry.redhat.io/compliance/openshift-compliance-must-gather-rhel8:latest"
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

	if instance.Spec.MustGatherImages == nil {
		instance.Spec.MustGatherImages = &supportv1alpha1.MustGatherImages{}
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.Standard == "" {
		std := defaultStandardMustGather
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_STANDARD"]; v != "" {
			std = v
		}
		instance.Spec.MustGatherImages.Standard = std
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.CNV == "" {
		cnv := defaultCNVMustGather
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_CNV"]; v != "" {
			cnv = v
		}
		instance.Spec.MustGatherImages.CNV = cnv
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.ODF == "" {
		odf := defaultODFMustGather
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_ODF"]; v != "" {
			odf = v
		}
		instance.Spec.MustGatherImages.ODF = odf
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.Logging == "" {
		instance.Spec.MustGatherImages.Logging = r.resolveImage("", "RELATED_IMAGE_MUST_GATHER_LOGGING", defaultLoggingMustGather)
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.ServiceMesh == "" {
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_SERVICE_MESH"]; v != "" {
			instance.Spec.MustGatherImages.ServiceMesh = v
			needsUpdate = true
		}
		// No default — app auto-detects version from installed CSV
	}
	if instance.Spec.MustGatherImages.Compliance == "" {
		instance.Spec.MustGatherImages.Compliance = r.resolveImage("", "RELATED_IMAGE_MUST_GATHER_COMPLIANCE", defaultComplianceMustGather)
		needsUpdate = true
	}
	if instance.Spec.MustGatherImages.MTC == "" {
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_MTC"]; v != "" {
			instance.Spec.MustGatherImages.MTC = v
			needsUpdate = true
		}
		// No default — app auto-detects version from installed CSV
	}
	if instance.Spec.MustGatherImages.GitOps == "" {
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_GITOPS"]; v != "" {
			instance.Spec.MustGatherImages.GitOps = v
			needsUpdate = true
		}
		// No default — app auto-detects version from installed CSV
	}
	if instance.Spec.MustGatherImages.Serverless == "" {
		if v := r.RelatedImages["RELATED_IMAGE_MUST_GATHER_SERVERLESS"]; v != "" {
			instance.Spec.MustGatherImages.Serverless = v
			needsUpdate = true
		}
		// No default — app auto-detects version from installed CSV
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
	oauthProxyImage := instance.Spec.OAuthProxyImage
	mgImages := instance.Spec.MustGatherImages
	clusterDomain := instance.Spec.ClusterDomain

	ns := instance.Namespace

	if err := r.reconcileServiceAccount(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("ServiceAccount: %v", err))
	}

	if err := r.reconcileClusterRoleBinding(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("ClusterRoleBinding: %v", err))
	}

	if err := r.reconcileCookieSecret(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Cookie Secret: %v", err))
	}

	if err := r.reconcileService(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Service: %v", err))
	}

	if err := r.reconcileAppMetrics(ctx, instance, ns); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Metrics: %v", err))
	}

	if err := r.reconcileDeployment(ctx, instance, ns, appImage, oauthProxyImage, clusterDomain, mgImages); err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Deployment: %v", err))
	}

	routeURL, routeHost, err := r.reconcileRoute(ctx, instance, ns)
	if err != nil {
		return ctrl.Result{}, r.setPhase(ctx, instance, "Failed", fmt.Sprintf("Route: %v", err))
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
	crbName := appName + "-" + ns
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crbName,
			Labels: labels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
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

func (r *OCPSupportWebReconciler) cleanupClusterResources(ctx context.Context, instance *supportv1alpha1.OCPSupportWeb) error {
	crbName := appName + "-" + instance.Namespace
	crb := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: crbName}, crb); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Delete(ctx, crb)
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

func (r *OCPSupportWebReconciler) reconcileDeployment(ctx context.Context, owner *supportv1alpha1.OCPSupportWeb, ns, appImage, oauthProxyImage, clusterDomain string, mgImages *supportv1alpha1.MustGatherImages) error {
	replicas := int32(1)

	appResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
	if owner.Spec.Resources != nil {
		appResources = *owner.Spec.Resources
	}

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
					Containers: []corev1.Container{
						{
							Name:            "oauth-proxy",
							Image:           oauthProxyImage,
							ImagePullPolicy: corev1.PullAlways,
							Args: []string{
								"--https-address=:8443",
								"--provider=openshift",
								"--openshift-service-account=" + appName,
								"--upstream=http://localhost:8080",
								"--tls-cert=/etc/tls/private/tls.crt",
								"--tls-key=/etc/tls/private/tls.key",
								"--cookie-secret-file=/etc/oauth/cookie-secret/cookie-secret",
								`--openshift-sar={"resource":"namespaces","verb":"create"}`,
								"--pass-user-headers=true",
							},
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
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
						{
							Name:            appName,
							Image:           appImage,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{Name: "CLUSTER_DOMAIN", Value: clusterDomain},
								{Name: "MUST_GATHER_IMAGE_DEFAULT", Value: mgImages.Standard},
								{Name: "MUST_GATHER_IMAGE_CNV", Value: mgImages.CNV},
								{Name: "MUST_GATHER_IMAGE_ODF", Value: mgImages.ODF},
								{Name: "MUST_GATHER_IMAGE_ACM", Value: mgImages.ACM},
								{Name: "MUST_GATHER_IMAGE_LOGGING", Value: mgImages.Logging},
								{Name: "MUST_GATHER_IMAGE_SERVICE_MESH", Value: mgImages.ServiceMesh},
								{Name: "MUST_GATHER_IMAGE_COMPLIANCE", Value: mgImages.Compliance},
								{Name: "MUST_GATHER_IMAGE_MTC", Value: mgImages.MTC},
								{Name: "MUST_GATHER_IMAGE_GITOPS", Value: mgImages.GitOps},
								{Name: "MUST_GATHER_IMAGE_SERVERLESS", Value: mgImages.Serverless},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
								{Name: "metrics", ContainerPort: 8081},
							},
							Resources: appResources,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: appName + "-tls",
								},
							},
						},
						{
							Name: "cookie-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: appName + "-cookie",
								},
							},
						},
					},
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

func (r *OCPSupportWebReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&supportv1alpha1.OCPSupportWeb{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
