package controller

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
	"prosimcorp.com/kuberbac/internal/globals"
)

// CheckNameSelector checks if the nameSelector has only one field filled
func (r *DynamicRoleBindingReconciler) CheckNameSelector(ctx context.Context, nameSelector *kuberbacv1alpha1.NameSelectorT) (err error) {

	// Check just only field is filled
	filledSelectorFields := 0

	if len(nameSelector.MatchList) > 0 {
		filledSelectorFields++
	}

	if nameSelector.MatchRegex.Expression != "" {
		filledSelectorFields++
	}

	if filledSelectorFields != 1 {
		err = fmt.Errorf("only one of the following fields is allowed as nameSelector: matchList, matchRegex")
	}

	return err
}

// CheckNamespaceSelector checks if the namespaceSelector has only one field filled
func (r *DynamicRoleBindingReconciler) CheckNamespaceSelector(ctx context.Context, namespaceSelector *kuberbacv1alpha1.NamespaceSelectorT) (err error) {

	// Check just only field is filled
	filledSelectorFields := 0

	if len(namespaceSelector.MatchLabels) > 0 {
		filledSelectorFields++
	}

	if len(namespaceSelector.MatchList) > 0 {
		filledSelectorFields++
	}

	if namespaceSelector.MatchRegex.Expression != "" {
		filledSelectorFields++
	}

	if filledSelectorFields != 1 {
		err = fmt.Errorf("only one of the following fields is allowed as namespaceSelector: matchLabels, matchList, matchRegex")
	}

	return err
}

// FilterNamespaceListBySelector returns a list of namespaces that match a namespaceSelector field
func (r *DynamicRoleBindingReconciler) FilterNamespaceListBySelector(ctx context.Context, namespaceList *corev1.NamespaceList, namespaceSelector *kuberbacv1alpha1.NamespaceSelectorT) (namespaces []string, err error) {

	// Check just only field is filled
	err = r.CheckNamespaceSelector(ctx, namespaceSelector)
	if err != nil {
		return namespaces, err
	}

	//
	matchRegex := &regexp.Regexp{}
	if namespaceSelector.MatchRegex.Expression != "" {
		matchRegex, err = regexp.Compile(namespaceSelector.MatchRegex.Expression)
		if err != nil {
			return namespaces, err
		}
	}

	//
	for _, namespace := range namespaceList.Items {

		// Check MatchLabels
		if len(namespaceSelector.MatchLabels) > 0 {

			if globals.IsSubset(namespaceSelector.MatchLabels, namespace.Labels) {
				namespaces = append(namespaces, namespace.Name)
			}
		}

		// Check MatchList
		if len(namespaceSelector.MatchList) > 0 {

			if slices.Contains(namespaceSelector.MatchList, namespace.Name) {
				namespaces = append(namespaces, namespace.Name)
			}
		}

		// Check MatchRegex
		if namespaceSelector.MatchRegex.Expression != "" {

			namespaceMatched := matchRegex.MatchString(namespace.Name)

			if !namespaceMatched && namespaceSelector.MatchRegex.Negative {
				namespaces = append(namespaces, namespace.Name)
				continue
			}

			if namespaceMatched && !namespaceSelector.MatchRegex.Negative {
				namespaces = append(namespaces, namespace.Name)
			}
		}

	}

	return namespaces, err
}

// GetServiceAccountsBySelectors TODO
func (r *DynamicRoleBindingReconciler) GetServiceAccountsBySelectors(ctx context.Context, filteredNamespaceList []string, nameSelector *kuberbacv1alpha1.NameSelectorT, namespaceSelector *kuberbacv1alpha1.NamespaceSelectorT) (result *corev1.ServiceAccountList, err error) {

	result = &corev1.ServiceAccountList{}

	tmpServiceAccountList := &corev1.ServiceAccountList{}
	err = r.Client.List(ctx, tmpServiceAccountList)
	if err != nil {
		return result, err
	}

	//
	matchRegex := &regexp.Regexp{}
	if nameSelector.MatchRegex.Expression != "" {
		matchRegex, err = regexp.Compile(nameSelector.MatchRegex.Expression)
		if err != nil {
			return result, err
		}
	}

	// Process ServiceAccounts discarding those from not-desired namespaces
	for _, serviceAccount := range tmpServiceAccountList.Items {

		// Ignore namespaces not present in desired list
		if !slices.Contains(filteredNamespaceList, serviceAccount.Namespace) {
			continue
		}

		// Matching by fixed list is preferred
		if len(nameSelector.MatchList) > 0 {
			if slices.Contains(nameSelector.MatchList, serviceAccount.Name) {
				result.Items = append(result.Items, serviceAccount)
			}
			continue
		}

		// Match by regex
		nameMatched := matchRegex.MatchString(serviceAccount.Name)

		if !nameMatched && nameSelector.MatchRegex.Negative {
			result.Items = append(result.Items, serviceAccount)
			continue
		}

		if nameMatched && !nameSelector.MatchRegex.Negative {
			result.Items = append(result.Items, serviceAccount)
		}

	}

	return result, err
}

// SyncTarget call Kubernetes API to actually perform actions over the resource
func (r *DynamicRoleBindingReconciler) SyncTarget(ctx context.Context, resource *kuberbacv1alpha1.DynamicRoleBinding) (err error) {

	// Check source.subject.kind is one of the valid values
	validKinds := []string{"ServiceAccount", "User", "Group"}
	if !slices.Contains(validKinds, resource.Spec.Source.Subject.Kind) {
		err = fmt.Errorf("source.subject.kind must be one of the following values: %s", strings.Join(validKinds, ", "))
		return err
	}

	// Enforce source.nameSelector has only one type of selector filled
	err = r.CheckNameSelector(ctx, &resource.Spec.Source.Subject.NameSelector)
	if err != nil {
		err = fmt.Errorf("source.subject.nameSelector is invalid: %s", err.Error())
		return err
	}

	// Check namespaceSelector does NOT exist for subjects other than ServiceAccount
	if slices.Contains([]string{"Group", "User"}, resource.Spec.Source.Subject.Kind) &&
		!reflect.ValueOf(resource.Spec.Source.Subject.NamespaceSelector).IsZero() {

		err = fmt.Errorf("namespaceSelector is not allowed for subjects other than ServiceAccount")
		return err
	}

	// Enforce namespaceSelector for ServiceAccount subjects
	if resource.Spec.Source.Subject.Kind == "ServiceAccount" &&
		reflect.ValueOf(resource.Spec.Source.Subject.NamespaceSelector).IsZero() {

		err = fmt.Errorf("namespaceSelector is required for ServiceAccount subjects")
		return err
	}

	// Get all the namespaces and filter them by namespaceSelector later
	namespaceList := &corev1.NamespaceList{}
	err = r.Client.List(ctx, namespaceList)
	if err != nil {
		return err
	}

	//
	sourceFilteredNamespaces, err := r.FilterNamespaceListBySelector(ctx, namespaceList, &resource.Spec.Source.Subject.NamespaceSelector)
	if err != nil {
		return err
	}
	targetFilteredNamespaces, err := r.FilterNamespaceListBySelector(ctx, namespaceList, &resource.Spec.Targets.NamespaceSelector)
	if err != nil {
		return err
	}

	// Create as many subjects as needed
	expandedSubjects := []rbacv1.Subject{}

	// Expand Group and User subjects
	if slices.Contains([]string{"Group", "User"}, resource.Spec.Source.Subject.Kind) {

		// MatchRegex nameSelector is not allowed for these subjects
		if !reflect.ValueOf(resource.Spec.Source.Subject.NameSelector.MatchRegex).IsZero() {
			err = fmt.Errorf("MatchRegex nameSelector is not allowed for subjects: Group, User")
			return err
		}

		//
		for _, listItem := range resource.Spec.Source.Subject.NameSelector.MatchList {
			expandedSubjects = append(expandedSubjects, rbacv1.Subject{
				Kind:     resource.Spec.Source.Subject.Kind,
				APIGroup: resource.Spec.Source.Subject.ApiGroup,
				Name:     listItem,
			})
		}
	}

	// Expand ServiceAccount subjects
	if resource.Spec.Source.Subject.Kind == "ServiceAccount" {

		serviceAccounts, err := r.GetServiceAccountsBySelectors(ctx, sourceFilteredNamespaces,
			&resource.Spec.Source.Subject.NameSelector, &resource.Spec.Source.Subject.NamespaceSelector)
		if err != nil {
			err = fmt.Errorf("error getting selected ServiceAccounts: %s", err.Error())
			return err
		}

		for _, serviceAccount := range serviceAccounts.Items {
			expandedSubjects = append(expandedSubjects, rbacv1.Subject{
				Kind:      "ServiceAccount",
				APIGroup:  resource.Spec.Source.Subject.ApiGroup,
				Name:      serviceAccount.Name,
				Namespace: serviceAccount.Namespace,
			})
		}
	}

	// Create a generic RoleBinding structure
	referenceAnnotations := map[string]string{
		"kuberbac.prosimcorp.com/owner-apiversion": resource.APIVersion,
		"kuberbac.prosimcorp.com/owner-kind":       resource.Kind,
		"kuberbac.prosimcorp.com/owner-name":       resource.ObjectMeta.Name,
		"kuberbac.prosimcorp.com/owner-namespace":  resource.ObjectMeta.Namespace,
	}

	if len(resource.ObjectMeta.Annotations) == 0 {
		resource.ObjectMeta.Labels = map[string]string{}
	}
	maps.Copy(resource.ObjectMeta.Annotations, referenceAnnotations)

	roleBindingResource := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        resource.ObjectMeta.Name,
			Annotations: resource.ObjectMeta.Annotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     resource.Spec.Source.ClusterRole,
		},
		Subjects: expandedSubjects,
	}

	// Create the RoleBinding resource on targeted namespaces
	for _, namespace := range targetFilteredNamespaces {
		roleBindingResource.SetNamespace(namespace)
		err = r.Client.Update(ctx, roleBindingResource.DeepCopy())
		if err != nil {
			log.Printf("error updating RoleBinding: %s", err.Error())
		}
	}

	// Get Rolebindings
	existentRoleBindingList := rbacv1.RoleBindingList{}
	err = r.Client.List(ctx, &existentRoleBindingList)
	if err != nil {
		return err
	}

	// For cleaning potential previous abandoned resources, get the list of namespaces
	// that are not reconciled in this loop to look for RoleBindings there
	targetNamespacesComplementaryList := slices.DeleteFunc(namespaceList.Items,
		func(namespace corev1.Namespace) bool {
			return slices.Contains(targetFilteredNamespaces, namespace.ObjectMeta.Name)
		},
	)

	// Get not targeted namespace list
	targetNamespacesComplementaryStrList := []string{}
	for _, namespace := range targetNamespacesComplementaryList {
		targetNamespacesComplementaryStrList = append(targetNamespacesComplementaryStrList, namespace.ObjectMeta.Name)
	}

	// Remove owned RoleBidings not defined in manifest
	for _, roleBinding := range existentRoleBindingList.Items {
		delete := false
		if globals.IsSubset(referenceAnnotations, roleBinding.Annotations) {
			delete = true
		}

		if delete && slices.Contains(targetNamespacesComplementaryStrList, roleBinding.Namespace) {
			err = r.Client.Delete(ctx, &roleBinding)
			if err != nil {
				err = fmt.Errorf("error deleting not needed rolebindings: %s", err.Error())

			}
		}
	}

	return err
}