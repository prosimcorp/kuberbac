package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/exp/maps"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
	"prosimcorp.com/kuberbac/internal/globals"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// parseSyncTimeError error message for invalid value on 'synchronization' parameter
	parseSyncTimeError = "can not parse the synchronization time from dynamicClusterRole: %s"
)

// GVKR represents a resource type inside Kubernetes
type GVKR struct {
	GVK         schema.GroupVersionKind
	Resource    string
	Subresource string

	//
	Namespaced  bool
	UsableVerbs []string // Intended for future use polishing resulting verbs
}

// PolicyRulesProcessorT represents the things done
// in the backstage to process PolicyRules
type PolicyRulesProcessorT struct {
	Context context.Context

	//
	Client          client.Client
	DiscoveryClient discovery.DiscoveryClient

	//
	ResourcesByGroup map[string][]GVKR
	ResourceList     []string
}

func NewPolicyRuleProcessor(context context.Context, client client.Client, discoveryClient discovery.DiscoveryClient) (prp PolicyRulesProcessorT, err error) {
	prp.Context = context
	prp.Client = client
	prp.DiscoveryClient = discoveryClient

	err = prp.SetResourcesByGroup()
	if err != nil {
		return prp, err
	}
	prp.SetResourceList()

	return prp, err
}

// SetResourcesByGroup retrieves all resources available in the cluster
// and store a map of groups with their resources inside it into the PolicyRulesProcessorT struct
func (p *PolicyRulesProcessorT) SetResourcesByGroup() (err error) {

	p.ResourcesByGroup = make(map[string][]GVKR)

	// Retrieve all types of resources available in the cluster
	_, apiGroupResourcesLists, err := p.DiscoveryClient.ServerGroupsAndResources()
	if err != nil {
		return err
	}

	// Process the resources and group them by API group
	for _, resourcesLists := range apiGroupResourcesLists {

		//
		groupVersion := strings.Split(resourcesLists.GroupVersion, "/")

		//
		group := ""
		version := groupVersion[0]

		if len(groupVersion) == 2 {
			group = groupVersion[0]
			version = groupVersion[1]
		}

		p.ResourcesByGroup[group] = []GVKR{}

		for _, apiResource := range resourcesLists.APIResources {

			resourceSubResource := strings.Split(apiResource.Name, "/")
			resource := resourceSubResource[0]
			subresource := ""
			if len(resourceSubResource) > 1 {
				subresource = strings.Join(resourceSubResource[1:], "/")
			}
			p.ResourcesByGroup[group] = append(p.ResourcesByGroup[group], GVKR{
				Resource:    resource,
				Subresource: subresource,
				GVK: schema.GroupVersionKind{
					Group:   group,
					Version: version,
					Kind:    apiResource.Kind,
				},
				Namespaced:  apiResource.Namespaced,
				UsableVerbs: apiResource.Verbs,
			})
		}
	}

	return err
}

// SetResourceList constructs a simple list of resources available in the cluster
// and store it into the PolicyRulesProcessorT struct
func (p *PolicyRulesProcessorT) SetResourceList() {
	for _, resList := range p.ResourcesByGroup {
		for _, res := range resList {
			if res.Subresource != "" {
				p.ResourceList = append(p.ResourceList, res.Resource+"/"+res.Subresource)
				continue
			}

			p.ResourceList = append(p.ResourceList, res.Resource)
		}
	}
}

// GetSurvivingVerbs returns allowed verbs that are not in the deny list
func (p *PolicyRulesProcessorT) GetSurvivingVerbs(allowVerbs []string, denyVerbs []string) (result []string) {
	tmpMap := map[string]int{}

	for _, allowVerbsVal := range allowVerbs { // list
		tmpMap[allowVerbsVal] = 1
	}

	for _, denyVerbsVal := range denyVerbs { // get
		if _, ok := tmpMap[denyVerbsVal]; !ok {
			continue
		}

		tmpMap[denyVerbsVal] = tmpMap[denyVerbsVal] + 1
	}

	for tmpMapKey, tmpMapVal := range tmpMap {
		if tmpMapVal == 1 {
			result = append(result, tmpMapKey)
		}
	}

	return result
}

// ExpandPolicyRules gets a list of PolicyRules and expands wildcard items to specific ones
func (p *PolicyRulesProcessorT) ExpandPolicyRules(policyRules []rbacv1.PolicyRule) (result []rbacv1.PolicyRule) {

	for _, policyRule := range policyRules {

		// No verbs? Kubernets will ignore you, so we will too
		if len(policyRule.Verbs) == 0 {
			continue
		}

		// Rules with NonResourceUrls can NOT come with APIGroups or Resources or ResourceNames
		if len(policyRule.NonResourceURLs) != 0 &&
			(len(policyRule.APIGroups) != 0 || len(policyRule.Resources) != 0 || len(policyRule.ResourceNames) != 0) {
			continue
		}

		// Rules without NonResourceUrls MUST come with APIgroups and Resources defined
		if len(policyRule.NonResourceURLs) == 0 &&
			(len(policyRule.APIGroups) == 0 || len(policyRule.Resources) == 0) {
			continue
		}

		// Rules with ResourceNames MUST come with Resources and APIGroups defined
		if len(policyRule.ResourceNames) != 0 &&
			(len(policyRule.APIGroups) == 0 || len(policyRule.Resources) == 0) {
			continue
		}

		//
		newPolicyRule := rbacv1.PolicyRule{}

		// 1. Expand groups in the PolicyRule.
		// Add all of them or user-specified ones.
		if slices.Contains(policyRule.APIGroups, "*") {
			for group := range p.ResourcesByGroup {
				newPolicyRule.APIGroups = append(newPolicyRule.APIGroups, group)
			}
		} else {
			for _, group := range policyRule.APIGroups {
				if _, ok := p.ResourcesByGroup[group]; ok {
					newPolicyRule.APIGroups = append(newPolicyRule.APIGroups, group)
				}
			}
		}

		// 2. Expand resources in the PolicyRule.
		// Add all of them or user-specified ones.
		if slices.Contains(policyRule.Resources, "*") {

			// Replace '*' with all resources owned by groups defined in the PolicyRule
			// Loop over defined groups, probe their existence, and get their probed resources
			for _, group := range newPolicyRule.APIGroups {

				if _, ok := p.ResourcesByGroup[group]; ok {

					for _, gvkr := range p.ResourcesByGroup[group] {

						if gvkr.Subresource != "" {
							newPolicyRule.Resources = append(newPolicyRule.Resources, gvkr.Resource+"/"+gvkr.Subresource)
							continue
						}

						newPolicyRule.Resources = append(newPolicyRule.Resources, gvkr.Resource)
					}
				}
			}
		} else {

			for _, resource := range policyRule.Resources {

				// Add only resources that exists
				if slices.Contains(p.ResourceList, resource) {
					newPolicyRule.Resources = append(newPolicyRule.Resources, resource)
				}
			}
		}

		// 2.1. This is a middle cleanup step after previous expansions
		// Delete groups that should NOT be there for the resources present in the PolicyRule
		// When the resource type is not found, delete it too
		newGroupList := []string{}
		for _, resource := range newPolicyRule.Resources {
			for _, group := range newPolicyRule.APIGroups {

				// Add group to marked-groups only when a resource type is found for that group in the huge map
				for _, gvkr := range p.ResourcesByGroup[group] {
					resourceType := strings.Split(resource, "/")[0]
					if strings.Compare(gvkr.Resource, resourceType) == 0 && !slices.Contains(newGroupList, group) {
						newGroupList = append(newGroupList, group)
						break
					}
				}
			}
		}
		newPolicyRule.APIGroups = newGroupList

		// 3. Add some fields as it
		newPolicyRule.ResourceNames = policyRule.ResourceNames
		newPolicyRule.NonResourceURLs = policyRule.NonResourceURLs

		// 4. Expand verbs in the PolicyRule.
		if slices.Contains(policyRule.Verbs, "*") {
			newPolicyRule.Verbs = []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}
		} else {
			newPolicyRule.Verbs = policyRule.Verbs
		}

		result = append(result, newPolicyRule)
	}

	return result
}

// StretchPolicyRules gets a list of complex PolicyRules and returns a new list with single resource per item
func (p *PolicyRulesProcessorT) StretchPolicyRules(policyRules []rbacv1.PolicyRule) (result []rbacv1.PolicyRule) {

	for _, policyRule := range policyRules {

		// Append rules with NonResourceURLs without expansion
		if len(policyRule.NonResourceURLs) > 0 {
			for _, url := range policyRule.NonResourceURLs {
				result = append(result, rbacv1.PolicyRule{
					NonResourceURLs: []string{url},
					Verbs:           policyRule.Verbs,
				})
			}
			continue
		}

		// Append the rest of the rules expanding them
		// We are checking that resource exists in a group
		for _, resource := range policyRule.Resources {

			for _, group := range policyRule.APIGroups {

				//
				resourceFound := false
				for _, gvkr := range p.ResourcesByGroup[group] {

					tmpResourceName := gvkr.Resource
					if gvkr.Subresource != "" {
						tmpResourceName += "/" + gvkr.Subresource
					}

					if strings.Compare(tmpResourceName, resource) == 0 {
						resourceFound = true
					}
				}

				if !resourceFound {
					continue
				}

				//
				if len(policyRule.ResourceNames) != 0 {
					for _, name := range policyRule.ResourceNames {
						result = append(result, rbacv1.PolicyRule{
							APIGroups:     []string{group},
							Resources:     []string{resource},
							ResourceNames: []string{name},
							Verbs:         policyRule.Verbs,
						})
					}
					continue
				}

				//
				result = append(result, rbacv1.PolicyRule{
					APIGroups: []string{group},
					Resources: []string{resource},
					Verbs:     policyRule.Verbs,
				})
			}
		}
	}

	return result
}

// GetMapFromStretchedPolicyRules return a map with the keys in the form of
// "group#resource#resourceName" or "nonresourceurl#url", and the value as PolicyRule
func (p *PolicyRulesProcessorT) GetMapFromStretchedPolicyRules(policyRules []rbacv1.PolicyRule) (result map[string]rbacv1.PolicyRule) {

	result = make(map[string]rbacv1.PolicyRule)

	for _, policyRule := range policyRules {

		// For NonResourceURLs rules
		if len(policyRule.NonResourceURLs) != 0 {

			nonResourceUrlMapKey := "nonresourceurl#" + policyRule.NonResourceURLs[0]

			if _, nonResourceUrlKeyFound := result[nonResourceUrlMapKey]; nonResourceUrlKeyFound {
				tmp := append(result[nonResourceUrlMapKey].Verbs, policyRule.Verbs...)
				slices.Sort(tmp)
				tmp = slices.Compact(tmp)

				result[nonResourceUrlMapKey] = rbacv1.PolicyRule{
					NonResourceURLs: policyRule.NonResourceURLs,
					Verbs:           tmp,
				}
				continue
			}

			result[nonResourceUrlMapKey] = policyRule

			continue
		}

		// For ResourceNames rules
		resourceKey := policyRule.APIGroups[0] + "#" + policyRule.Resources[0] + "#"
		if len(policyRule.ResourceNames) != 0 {
			resourceKey += policyRule.ResourceNames[0]
		}

		if _, resourceKeyFound := result[resourceKey]; resourceKeyFound {

			tmp := append(result[resourceKey].Verbs, policyRule.Verbs...)
			slices.Sort(tmp)
			tmp = slices.Compact(tmp)

			result[resourceKey] = rbacv1.PolicyRule{
				APIGroups:     policyRule.APIGroups,
				Resources:     policyRule.Resources,
				ResourceNames: policyRule.ResourceNames,
				Verbs:         tmp,
			}
			continue
		}

		result[resourceKey] = policyRule
	}
	return result
}

// EvaluateSpecialCases checks for special cases in the PolicyRules maps
// and returns the resulting map with them evaluated
func (p *PolicyRulesProcessorT) EvaluateSpecialCases(allowMap, denyMap map[string]rbacv1.PolicyRule) (result map[string]rbacv1.PolicyRule, err error) {

	for denyMapkey, policyRule := range denyMap {
		if strings.HasPrefix(denyMapkey, "nonresourceurl") {
			continue
		}

		// Generic resource found, ignore it
		parts := strings.Split(denyMapkey, "#")
		if parts[2] == "" {
			continue
		}

		// We found a deny rule acting on a Resource with ResourceName,
		// Find the Resources without ResourceName in the allow map
		// and add all the resource names minus the ones in the deny rule
		key := strings.Join(parts[:2], "#") + "#"
		if _, ok := allowMap[key]; ok {

			// Find the GVKR for the resource allocated in deny
			tmpGvkr := GVKR{}
			coreResourceType := strings.Split(policyRule.Resources[0], "/")[0]
			for _, gvkr := range p.ResourcesByGroup[policyRule.APIGroups[0]] {
				if gvkr.Resource == coreResourceType {
					tmpGvkr = gvkr
				}
			}

			// Get a list of all the resources of the same type
			sourceObjectList := &unstructured.UnstructuredList{}
			sourceObjectList.SetGroupVersionKind(tmpGvkr.GVK)
			err = p.Client.List(p.Context, sourceObjectList, []client.ListOption{}...)
			if err != nil {
				return result, err
			}

			for _, sourceObject := range sourceObjectList.Items {

				allowMap[key+sourceObject.GetName()] = rbacv1.PolicyRule{
					APIGroups:     allowMap[key].APIGroups,
					Resources:     allowMap[key].Resources,
					ResourceNames: []string{sourceObject.GetName()},
					Verbs:         allowMap[key].Verbs,
				}
			}

			delete(allowMap, key)
		}
	}

	result = allowMap
	return result, err
}

// EvaluatePolicyRules compares the allow and deny PolicyRule maps and returns the resulting map
func (p *PolicyRulesProcessorT) EvaluatePolicyRules(allowMap, denyMap map[string]rbacv1.PolicyRule) (result map[string]rbacv1.PolicyRule, err error) {

	for denyMapKey, policyRule := range denyMap {

		// NonResourceURLs rules
		if strings.HasPrefix(denyMapKey, "nonresourceurl") {

			// Wildcard deny rule found for a NonResourceURLs,
			// Treat verbs for all allow rules that match the prefix
			if strings.HasSuffix(denyMapKey, "*") {

				nonResourceUrlPrefix := strings.TrimSuffix(denyMapKey, "*")

				for allowMapKey, _ := range allowMap {

					if strings.HasPrefix(allowMapKey, nonResourceUrlPrefix) {
						tmpPolicyRule := allowMap[allowMapKey]
						tmpPolicyRule.Verbs = p.GetSurvivingVerbs(allowMap[allowMapKey].Verbs, policyRule.Verbs)
						allowMap[allowMapKey] = tmpPolicyRule
					}

					if len(allowMap[allowMapKey].Verbs) == 0 {
						delete(allowMap, allowMapKey)
					}
				}
				continue
			}

			// Treat the verbs on all allow rules that match the exact NonResourceURLs
			tmpPolicyRule := allowMap[denyMapKey]
			tmpPolicyRule.Verbs = p.GetSurvivingVerbs(allowMap[denyMapKey].Verbs, policyRule.Verbs)
			allowMap[denyMapKey] = tmpPolicyRule

			if len(allowMap[denyMapKey].Verbs) == 0 {
				delete(allowMap, denyMapKey)
			}

			continue
		}

		denyMapKeyParts := strings.Split(denyMapKey, "#")

		// Deny rule found for a Resouce NOT defining a ResourceName,
		// Treat verbs for all allow rules that match the prefix
		if denyMapKeyParts[2] == "" {
			for allowMapKey, _ := range allowMap {
				if strings.HasPrefix(allowMapKey, denyMapKey) {
					tmpPolicyRule := allowMap[allowMapKey]
					tmpPolicyRule.Verbs = p.GetSurvivingVerbs(allowMap[allowMapKey].Verbs, policyRule.Verbs)
					allowMap[allowMapKey] = tmpPolicyRule
				}

				if len(allowMap[allowMapKey].Verbs) == 0 {
					delete(allowMap, allowMapKey)
				}
			}
			continue
		}

		// Deny rule found for a Resouce DO defining a ResourceName,
		// Treat verbs for all allow rules that match the prefix
		if denyMapKeyParts[2] != "" {
			if _, ok := allowMap[denyMapKey]; ok {
				tmpPolicyRule := allowMap[denyMapKey]
				tmpPolicyRule.Verbs = p.GetSurvivingVerbs(allowMap[denyMapKey].Verbs, policyRule.Verbs)
				allowMap[denyMapKey] = tmpPolicyRule

				if len(allowMap[denyMapKey].Verbs) == 0 {
					delete(allowMap, denyMapKey)
				}
			}
		}
	}

	result = allowMap

	return result, err
}

// SplitPolicyRules separates PolicyRules into two lists: clusterScopedRules and namespaceScopedRules
func (p *PolicyRulesProcessorT) SplitPolicyRules(policyRules []rbacv1.PolicyRule) (clusterScopedRules, namespaceScopedRules []rbacv1.PolicyRule) {

	for _, policyRule := range policyRules {

		// Look for current PolicyRule in the resourcesByGroup map
		for _, resource := range p.ResourcesByGroup[policyRule.APIGroups[0]] {

			//
			resourceName := resource.Resource
			if resource.Subresource != "" {
				resourceName += "/" + resource.Subresource
			}

			// Ignore when it is not the correct resource
			if policyRule.Resources[0] != resourceName {
				continue
			}

			// Add to the corresponding list
			if resource.Namespaced {
				namespaceScopedRules = append(namespaceScopedRules, policyRule)
			} else {
				clusterScopedRules = append(clusterScopedRules, policyRule)
			}

			break
		}
	}

	return clusterScopedRules, namespaceScopedRules
}

// GetSyncTime return the spec.synchronization.time as duration, or default time on failures
func (r *DynamicClusterRoleReconciler) GetSyncTime(resource *kuberbacv1alpha1.DynamicClusterRole) (syncTime time.Duration, err error) {

	syncTime, err = time.ParseDuration(resource.Spec.Synchronization.Time)
	if err != nil {
		err = fmt.Errorf(parseSyncTimeError, resource.Name)
		return syncTime, err
	}

	return syncTime, err
}

// SyncTarget call Kubernetes API to actually perform actions over the resource
func (r *DynamicClusterRoleReconciler) SyncTarget(ctx context.Context, resource *kuberbacv1alpha1.DynamicClusterRole) (err error) {

	policyRulesProcessor, err := NewPolicyRuleProcessor(ctx, r.Client, r.DiscoveryClient)
	if err != nil {
		return fmt.Errorf("error generating PolicyRulesProcessor: %s", err.Error())
	}

	// Transform '*' symbols with actual things
	expandedAllowList := policyRulesProcessor.ExpandPolicyRules(resource.Spec.Allow)
	expandedDenyList := policyRulesProcessor.ExpandPolicyRules(resource.Spec.Deny)

	// Stretch policy rules to a single resource per item
	stretchAllowList := policyRulesProcessor.StretchPolicyRules(expandedAllowList)
	stretchDenyList := policyRulesProcessor.StretchPolicyRules(expandedDenyList)

	// Craft a map with stretched policy rules. Its keys are created as unique identifiers.
	// This is done to increase performance when evaluating the rules.
	allowMap := policyRulesProcessor.GetMapFromStretchedPolicyRules(stretchAllowList)
	denyMap := policyRulesProcessor.GetMapFromStretchedPolicyRules(stretchDenyList)

	//
	allowMap, err = policyRulesProcessor.EvaluateSpecialCases(allowMap, denyMap)
	if err != nil {
		return fmt.Errorf("error evaluating especial cases: %s", err.Error())
	}

	//
	result, err := policyRulesProcessor.EvaluatePolicyRules(allowMap, denyMap)
	if err != nil {
		return fmt.Errorf("error evaluating allow and deny maps: %s", err.Error())
	}

	// Create a list of ClusterRoles to be created.
	// We assume always only one ClusterRole, but this will be transformed into two when asked to separate scopes.
	clusterRoles := []rbacv1.ClusterRole{}

	referenceAnnotations := map[string]string{
		"kuberbac.prosimcorp.com/owner-apiversion": resource.APIVersion,
		"kuberbac.prosimcorp.com/owner-kind":       resource.Kind,
		"kuberbac.prosimcorp.com/owner-name":       resource.ObjectMeta.Name,
		"kuberbac.prosimcorp.com/owner-namespace":  resource.ObjectMeta.Namespace,
	}

	if len(resource.Spec.Target.Annotations) == 0 {
		resource.Spec.Target.Annotations = map[string]string{}
	}

	clusterRoleResource := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:        resource.Spec.Target.Name,
			Annotations: referenceAnnotations,
			Labels:      resource.Spec.Target.Labels,
		},
		Rules: maps.Values(result),
		// TODO: Implement AggregationRules later
	}
	clusterRoles = append(clusterRoles, clusterRoleResource)

	//
	if resource.Spec.Target.SeparateScopes {
		clusterScopedRules, namespaceScopedRules := policyRulesProcessor.SplitPolicyRules(maps.Values(result))

		// Assume first ClusterRole as clusterScoped
		clusterRoles[0].Rules = clusterScopedRules
		clusterRoles[0].Name = resource.Spec.Target.Name + "-cluster"

		// Create a new ClusterRole for namespaceScoped
		clusterRoles = append(clusterRoles, *clusterRoleResource.DeepCopy())
		clusterRoles[1].Rules = namespaceScopedRules
		clusterRoles[1].Name = resource.Spec.Target.Name + "-namespace"
	}

	//
	for _, clusterRole := range clusterRoles {
		err = r.Client.Update(ctx, &clusterRole)
		if err != nil {
			err = fmt.Errorf("error updating ClusterRole: %s", err.Error())
			break
		}
	}

	return err
}

// DeleteTargets deletes all the ClusterRoles that are owned by the DynamicClusterRole resource
func (r *DynamicClusterRoleReconciler) DeleteTargets(ctx context.Context, resource *kuberbacv1alpha1.DynamicClusterRole) (err error) {

	var allErrors []error

	// Create a generic ClusterRole structure
	referenceAnnotations := map[string]string{
		"kuberbac.prosimcorp.com/owner-apiversion": resource.APIVersion,
		"kuberbac.prosimcorp.com/owner-kind":       resource.Kind,
		"kuberbac.prosimcorp.com/owner-name":       resource.ObjectMeta.Name,
		"kuberbac.prosimcorp.com/owner-namespace":  resource.ObjectMeta.Namespace,
	}

	// Get ClusterRole objects and delete those with reference annotations
	clusterRoleList := rbacv1.ClusterRoleList{}
	err = r.Client.List(ctx, &clusterRoleList)
	if err != nil {
		return err
	}

	for _, clusterRole := range clusterRoleList.Items {

		if globals.IsSubset(referenceAnnotations, clusterRole.Annotations) {
			err = r.Client.Delete(ctx, &clusterRole)
			if err = client.IgnoreNotFound(err); err != nil {
				allErrors = append(allErrors, fmt.Errorf("error deleting ClusterRoleBinding: %s", err.Error()))
			}
		}
	}

	return errors.Join(allErrors...)
}
