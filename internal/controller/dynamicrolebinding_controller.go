/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/client-go/discovery"

	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
)

// DynamicRoleBindingReconciler reconciles a DynamicRoleBinding object
type DynamicRoleBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// TODO
	DiscoveryClient discovery.DiscoveryClient
}

// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicrolebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicrolebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete;bind;escalate
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *DynamicRoleBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	//1. Get the content of the Patch
	dynamicRoleBindingResource := &kuberbacv1alpha1.DynamicRoleBinding{}
	err = r.Get(ctx, req.NamespacedName, dynamicRoleBindingResource)

	// 2. Check existence on the cluster
	if err != nil {

		// 2.1 It does NOT exist: manage removal
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Info(fmt.Sprintf(resourceNotFoundError, DynamicRoleBindingResourceType, req.NamespacedName))
			return result, err
		}

		// 2.2 Failed to get the resource, requeue the request
		logger.Info(fmt.Sprintf(resourceSyncTimeRetrievalError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
		return result, err
	}

	// 3. Check if the DynamicClusterRole instance is marked to be deleted: indicated by the deletion timestamp being set
	if !dynamicRoleBindingResource.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(dynamicRoleBindingResource, resourceFinalizer) {

			// Delete all created targets
			err = r.DeleteTargets(ctx, dynamicRoleBindingResource)
			if err != nil {
				logger.Info(fmt.Sprintf(resourceTargetsDeleteError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
				return result, err
			}

			// Remove the finalizers on CR
			controllerutil.RemoveFinalizer(dynamicRoleBindingResource, resourceFinalizer)
			err = r.Update(ctx, dynamicRoleBindingResource)
			if err != nil {
				logger.Info(fmt.Sprintf(resourceFinalizersUpdateError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
			}
		}
		result = ctrl.Result{}
		err = nil
		return result, err
	}

	// 4. Add finalizer to the DynamicClusterRole CR
	if !controllerutil.ContainsFinalizer(dynamicRoleBindingResource, resourceFinalizer) {
		controllerutil.AddFinalizer(dynamicRoleBindingResource, resourceFinalizer)
		err = r.Update(ctx, dynamicRoleBindingResource)
		if err != nil {
			return result, err
		}
	}

	// 5. Update the status before the requeue
	defer func() {
		err = r.Status().Update(ctx, dynamicRoleBindingResource)
		if err != nil {
			logger.Info(fmt.Sprintf(resourceConditionUpdateError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
		}
	}()

	// 6. Schedule periodical request
	RequeueTime, err := time.ParseDuration(dynamicRoleBindingResource.Spec.Synchronization.Time)
	if err != nil {
		logger.Info(fmt.Sprintf(resourceSyncTimeRetrievalError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
		return result, err
	}
	result = ctrl.Result{
		RequeueAfter: RequeueTime,
	}

	// 7. The Patch CR already exist: manage the update
	err = r.SyncTarget(ctx, dynamicRoleBindingResource)
	if err != nil {
		r.UpdateConditionKubernetesApiCallFailure(dynamicRoleBindingResource)
		logger.Info(fmt.Sprintf(syncTargetError, DynamicRoleBindingResourceType, req.NamespacedName, err.Error()))
		return result, err
	}

	// 8. Success, update the status
	r.UpdateConditionSuccess(dynamicRoleBindingResource)

	logger.Info(fmt.Sprintf(scheduleSynchronization, DynamicRoleBindingResourceType, req.NamespacedName, result.RequeueAfter.String()))

	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicRoleBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuberbacv1alpha1.DynamicRoleBinding{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
