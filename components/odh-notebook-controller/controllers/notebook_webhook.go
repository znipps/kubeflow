/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"net/http"

	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/mutate-notebook-v1,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=notebooks.opendatahub.io,admissionReviewVersions=v1

// NotebookWebhook holds the webhook configuration.
type NotebookWebhook struct {
	Client      client.Client
	decoder     *admission.Decoder
	OAuthConfig OAuthConfig
}

// InjectOAuthProxy injects the OAuth proxy sidecar container in the Notebook
// spec
func InjectOAuthProxy(notebook *nbv1.Notebook, oauth OAuthConfig) error {
	// https://pkg.go.dev/k8s.io/api/core/v1#Container
	proxyContainer := corev1.Container{
		Name:            "oauth-proxy",
		Image:           oauth.ProxyImage,
		ImagePullPolicy: corev1.PullAlways,
		Env: []corev1.EnvVar{{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		}},
		Args: []string{
			"--provider=openshift",
			"--https-address=:8443",
			"--http-address=",
			"--openshift-service-account=" + notebook.Name,
			"--cookie-secret-file=/etc/oauth/config/cookie_secret",
			"--cookie-expire=24h0m0s",
			"--tls-cert=/etc/tls/private/tls.crt",
			"--tls-key=/etc/tls/private/tls.key",
			"--upstream=http://localhost:8888",
			"--upstream-ca=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
			"--skip-auth-regex=^/api$",
			"--email-domain=*",
			"--skip-provider-button",
			`--openshift-sar={"verb":"get","resource":"notebooks","resourceAPIGroup":"kubeflow.org",` +
				`"resourceName":"` + notebook.Name + `","namespace":"$(NAMESPACE)"}`,
		},
		Ports: []corev1.ContainerPort{{
			Name:          OAuthServicePortName,
			ContainerPort: 8443,
			Protocol:      corev1.ProtocolTCP,
		}},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/oauth/healthz",
					Port:   intstr.FromString(OAuthServicePortName),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 30,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/oauth/healthz",
					Port:   intstr.FromString(OAuthServicePortName),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"cpu":    resource.MustParse("100m"),
				"memory": resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				"cpu":    resource.MustParse("100m"),
				"memory": resource.MustParse("64Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "oauth-config",
				MountPath: "/etc/oauth/config",
			},
			{
				Name:      "tls-certificates",
				MountPath: "/etc/tls/private",
			},
		},
	}

	// Add the sidecar container to the notebook
	notebookContainers := &notebook.Spec.Template.Spec.Containers
	proxyContainerExists := false
	for index, container := range *notebookContainers {
		if container.Name == "oauth-proxy" {
			(*notebookContainers)[index] = proxyContainer
			proxyContainerExists = true
			break
		}
	}
	if !proxyContainerExists {
		*notebookContainers = append(*notebookContainers, proxyContainer)
	}

	// Add the OAuth configuration volume:
	// https://pkg.go.dev/k8s.io/api/core/v1#Volume
	notebookVolumes := &notebook.Spec.Template.Spec.Volumes
	oauthVolumeExists := false
	oauthVolume := corev1.Volume{
		Name: "oauth-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  notebook.Name + "-oauth-config",
				DefaultMode: pointer.Int32Ptr(420),
			},
		},
	}
	for index, volume := range *notebookVolumes {
		if volume.Name == "oauth-config" {
			(*notebookVolumes)[index] = oauthVolume
			oauthVolumeExists = true
			break
		}
	}
	if !oauthVolumeExists {
		*notebookVolumes = append(*notebookVolumes, oauthVolume)
	}

	// Add the TLS certificates volume:
	// https://pkg.go.dev/k8s.io/api/core/v1#Volume
	tlsVolumeExists := false
	tlsVolume := corev1.Volume{
		Name: "tls-certificates",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  notebook.Name + "-tls",
				DefaultMode: pointer.Int32Ptr(420),
			},
		},
	}
	for index, volume := range *notebookVolumes {
		if volume.Name == "tls-certificates" {
			(*notebookVolumes)[index] = tlsVolume
			tlsVolumeExists = true
			break
		}
	}
	if !tlsVolumeExists {
		*notebookVolumes = append(*notebookVolumes, tlsVolume)
	}

	// Set a dedicated service account, do not use default
	notebook.Spec.Template.Spec.ServiceAccountName = notebook.Name
	return nil
}

// Handle transforms the Notebook objects.
func (w *NotebookWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	notebook := &nbv1.Notebook{}

	err := w.decoder.Decode(req, notebook)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if OAuthInjectionIsEnabled(*notebook) {
		err = InjectOAuthProxy(notebook, w.OAuthConfig)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	} else {
		return admission.Allowed("Not injecting OAuth proxy")
	}

	marshaledNotebook, err := json.Marshal(notebook)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledNotebook)
}

// InjectDecoder injects the decoder.
func (w *NotebookWebhook) InjectDecoder(d *admission.Decoder) error {
	w.decoder = d
	return nil
}
