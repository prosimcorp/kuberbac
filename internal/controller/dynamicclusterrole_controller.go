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

	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
)

// DynamicClusterRoleReconciler reconciles a DynamicClusterRole object
type DynamicClusterRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// TODO
	DiscoveryClient discovery.DiscoveryClient
}

// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicclusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicclusterroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kuberbac.prosimcorp.com,resources=dynamicclusterroles/finalizers,verbs=update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;patch;delete;bind;escalate
// +kubebuilder:rbac:groups="*",resources="*",verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *DynamicClusterRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	//1. Get the content of the Patch
	dynamicClusterRoleResource := &kuberbacv1alpha1.DynamicClusterRole{}
	err = r.Get(ctx, req.NamespacedName, dynamicClusterRoleResource)

	// 2. Check existence on the cluster
	if err != nil {

		// 2.1 It does NOT exist: manage removal
		if err = client.IgnoreNotFound(err); err == nil {
			logger.Info(fmt.Sprintf(resourceNotFoundError, DynamicClusterRoleResourceType, req.NamespacedName))
			return result, err
		}

		// 2.2 Failed to get the resource, requeue the request
		logger.Info(fmt.Sprintf(resourceSyncTimeRetrievalError, DynamicClusterRoleResourceType, req.NamespacedName, err.Error()))
		return result, err
	}

	// 3. Check if the DynamicClusterRole instance is marked to be deleted: indicated by the deletion timestamp being set
	if !dynamicClusterRoleResource.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(dynamicClusterRoleResource, patchFinalizer) {
			// Remove the finalizers on Patch CR
			controllerutil.RemoveFinalizer(dynamicClusterRoleResource, patchFinalizer)
			err = r.Update(ctx, dynamicClusterRoleResource)
			if err != nil {
				logger.Info(fmt.Sprintf(resourceFinalizersUpdateError, DynamicClusterRoleResourceType, req.NamespacedName, err.Error()))
			}
		}
		result = ctrl.Result{}
		err = nil
		return result, err
	}

	// 4. Add finalizer to the DynamicClusterRole CR
	if !controllerutil.ContainsFinalizer(dynamicClusterRoleResource, patchFinalizer) {
		controllerutil.AddFinalizer(dynamicClusterRoleResource, patchFinalizer)
		err = r.Update(ctx, dynamicClusterRoleResource)
		if err != nil {
			return result, err
		}
	}

	// 5. Update the status before the requeue
	defer func() {
		err = r.Status().Update(ctx, dynamicClusterRoleResource)
		if err != nil {
			logger.Info(fmt.Sprintf(resourceConditionUpdateError, DynamicClusterRoleResourceType, req.NamespacedName, err.Error()))
		}
	}()

	// 6. Schedule periodical request
	RequeueTime, err := time.ParseDuration(dynamicClusterRoleResource.Spec.Synchronization.Time)
	if err != nil {
		logger.Info(fmt.Sprintf(resourceSyncTimeRetrievalError, DynamicClusterRoleResourceType, req.NamespacedName, err.Error()))
		return result, err
	}
	result = ctrl.Result{
		RequeueAfter: RequeueTime,
	}

	// 7. The Patch CR already exist: manage the update
	err = r.SyncTarget(ctx, dynamicClusterRoleResource)
	if err != nil {
		r.UpdateConditionKubernetesApiCallFailure(dynamicClusterRoleResource)
		logger.Info(fmt.Sprintf(syncTargetError, DynamicClusterRoleResourceType, req.NamespacedName, err.Error()))
		return result, err
	}

	// 8. Success, update the status
	r.UpdateConditionSuccess(dynamicClusterRoleResource)

	logger.Info(fmt.Sprintf(scheduleSynchronization, DynamicClusterRoleResourceType, req.NamespacedName, result.RequeueAfter.String()))
	return result, err
}

// SetupWithManager sets up the controller with the Manager.
// Ref: https://github.com/kubernetes-sigs/kubebuilder/issues/618
func (r *DynamicClusterRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuberbacv1alpha1.DynamicClusterRole{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
