/*
Copyright 2022 Strangelove Ventures LLC.

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

// Package v1 contains API Schema definitions for the cosmos v1 API group
// +kubebuilder:object:generate=true
// +groupName=cosmos.strange.love
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "cosmos.strange.love", Version: "v1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// CanaryCosmosFullNodeKind is the alternate kind under which the same Go types
// can be registered. The canary operator build runs the identical reconcile
// code against this kind so it can coexist with an operator managing
// CosmosFullNode objects - the two kinds never share objects or status writes.
const CanaryCosmosFullNodeKind = "CanaryCosmosFullNode"

// CanaryAddToScheme registers the CosmosFullNode Go types under the
// CanaryCosmosFullNode kind INSTEAD of CosmosFullNode. Controllers built with
// For(&CosmosFullNode{}) then watch, fetch, own and write
// canarycosmosfullnodes resources exclusively: watches, owner references,
// status subresource writes and client gets all resolve the GVK through the
// scheme. Registering the original kind too is not possible - a Go type maps
// to exactly one GVK per scheme.
func CanaryAddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypeWithName(GroupVersion.WithKind(CanaryCosmosFullNodeKind), &CosmosFullNode{})
	s.AddKnownTypeWithName(GroupVersion.WithKind(CanaryCosmosFullNodeKind+"List"), &CosmosFullNodeList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
