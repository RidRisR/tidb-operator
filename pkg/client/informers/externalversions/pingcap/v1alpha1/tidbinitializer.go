// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	pingcapv1alpha1 "github.com/pingcap/tidb-operator/pkg/apis/pingcap/v1alpha1"
	versioned "github.com/pingcap/tidb-operator/pkg/client/clientset/versioned"
	internalinterfaces "github.com/pingcap/tidb-operator/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/pingcap/tidb-operator/pkg/client/listers/pingcap/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// TidbInitializerInformer provides access to a shared informer and lister for
// TidbInitializers.
type TidbInitializerInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.TidbInitializerLister
}

type tidbInitializerInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewTidbInitializerInformer constructs a new informer for TidbInitializer type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewTidbInitializerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredTidbInitializerInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredTidbInitializerInformer constructs a new informer for TidbInitializer type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredTidbInitializerInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PingcapV1alpha1().TidbInitializers(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PingcapV1alpha1().TidbInitializers(namespace).Watch(options)
			},
		},
		&pingcapv1alpha1.TidbInitializer{},
		resyncPeriod,
		indexers,
	)
}

func (f *tidbInitializerInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredTidbInitializerInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *tidbInitializerInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&pingcapv1alpha1.TidbInitializer{}, f.defaultInformer)
}

func (f *tidbInitializerInformer) Lister() v1alpha1.TidbInitializerLister {
	return v1alpha1.NewTidbInitializerLister(f.Informer().GetIndexer())
}