package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (in *OCPSupportWebSpec) DeepCopyInto(out *OCPSupportWebSpec) {
	*out = *in
	if in.Route != nil {
		in, out := &in.Route, &out.Route
		*out = new(RouteSpec)
		**out = **in
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.OAuthProxyResources != nil {
		in, out := &in.OAuthProxyResources, &out.OAuthProxyResources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.AllowedGroups != nil {
		in, out := &in.AllowedGroups, &out.AllowedGroups
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *OCPSupportWebStatus) DeepCopyInto(out *OCPSupportWebStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *SupportGatherSpec) DeepCopyInto(out *SupportGatherSpec) {
	*out = *in
	if in.GatherTypes != nil {
		in, out := &in.GatherTypes, &out.GatherTypes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ResourceTypes != nil {
		in, out := &in.ResourceTypes, &out.ResourceTypes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IncludeLogs != nil {
		in, out := &in.IncludeLogs, &out.IncludeLogs
		*out = new(bool)
		**out = **in
	}
	if in.Upload != nil {
		in, out := &in.Upload, &out.Upload
		*out = new(UploadSpec)
		**out = **in
	}
}

func (in *SupportGatherStatus) DeepCopyInto(out *SupportGatherStatus) {
	*out = *in
	if in.CompletionTime != nil {
		in, out := &in.CompletionTime, &out.CompletionTime
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}
