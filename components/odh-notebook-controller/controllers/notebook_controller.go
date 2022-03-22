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
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AnnotationInjectOAuth = "notebooks.opendatahub.io/inject-oauth"
)

// OpenshiftNotebookReconciler holds the controller configuration.
type OpenshiftNotebookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// ClusterRole permissions

// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks,verbs=get;list;watch
// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/status,verbs=get
// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/finalizers,verbs=update
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;secrets,verbs=get;list;watch;create;update;patch

// OAuthInjectionIsEnabled returns true if the oauth sidecar injection
// annotation is present in the notebook
func OAuthInjectionIsEnabled(notebook nbv1.Notebook) bool {
	if notebook.Annotations[AnnotationInjectOAuth] != "" {
		result, _ := strconv.ParseBool(notebook.Annotations[AnnotationInjectOAuth])
		return result
	} else {
		return false
	}
}

// CompareNotebooks checks if two notebooks are equal, if not return false
func CompareNotebooks(nb1 nbv1.Notebook, nb2 nbv1.Notebook) bool {
	return reflect.DeepEqual(nb1.ObjectMeta.Labels, nb2.ObjectMeta.Labels) &&
		reflect.DeepEqual(nb1.ObjectMeta.Annotations, nb2.ObjectMeta.Annotations) &&
		reflect.DeepEqual(nb1.Spec, nb2.Spec)
}

// Reconcile performs the reconciling of the Openshift objects for a Kubeflow
// Notebook.
func (r *OpenshiftNotebookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Initialize logger format
	log := r.Log.WithValues("notebook", req.Name, "namespace", req.Namespace)

	// Get the notebook object when a reconciliation event is triggered (create,
	// update, delete)
	notebook := &nbv1.Notebook{}
	err := r.Get(ctx, req.NamespacedName, notebook)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Stop Notebook reconciliation")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Unable to fetch the Notebook")
		return ctrl.Result{}, err
	}

	// Create the objects required by the OAuth proxy sidecar (see
	// notebook_oauth.go file)
	if OAuthInjectionIsEnabled(*notebook) {
		// Call the OAuth Service Account reconciler
		err = r.ReconcileOAuthServiceAccount(notebook, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Call the OAuth Service reconciler
		err = r.ReconcileOAuthService(notebook, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Call the OAuth Secret reconciler
		err = r.ReconcileOAuthSecret(notebook, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Call the OAuth Route reconciler
		err = r.ReconcileOAuthRoute(notebook, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Call the route reconciler (see notebook_route.go file)
		err = r.ReconcileRoute(notebook, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenshiftNotebookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&nbv1.Notebook{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{})

	err := builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
