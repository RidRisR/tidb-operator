// Copyright PingCAP, Inc.
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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/pingcap/tidb-operator/pkg/apis/pingcap/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeCompactBackups implements CompactBackupInterface
type FakeCompactBackups struct {
	Fake *FakePingcapV1alpha1
	ns   string
}

var compactbackupsResource = v1alpha1.SchemeGroupVersion.WithResource("compactbackups")

var compactbackupsKind = v1alpha1.SchemeGroupVersion.WithKind("CompactBackup")

// Get takes name of the compactBackup, and returns the corresponding compactBackup object, and an error if there is any.
func (c *FakeCompactBackups) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.CompactBackup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(compactbackupsResource, c.ns, name), &v1alpha1.CompactBackup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.CompactBackup), err
}

// List takes label and field selectors, and returns the list of CompactBackups that match those selectors.
func (c *FakeCompactBackups) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.CompactBackupList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(compactbackupsResource, compactbackupsKind, c.ns, opts), &v1alpha1.CompactBackupList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.CompactBackupList{ListMeta: obj.(*v1alpha1.CompactBackupList).ListMeta}
	for _, item := range obj.(*v1alpha1.CompactBackupList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested compactBackups.
func (c *FakeCompactBackups) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(compactbackupsResource, c.ns, opts))

}

// Create takes the representation of a compactBackup and creates it.  Returns the server's representation of the compactBackup, and an error, if there is any.
func (c *FakeCompactBackups) Create(ctx context.Context, compactBackup *v1alpha1.CompactBackup, opts v1.CreateOptions) (result *v1alpha1.CompactBackup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(compactbackupsResource, c.ns, compactBackup), &v1alpha1.CompactBackup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.CompactBackup), err
}

// Update takes the representation of a compactBackup and updates it. Returns the server's representation of the compactBackup, and an error, if there is any.
func (c *FakeCompactBackups) Update(ctx context.Context, compactBackup *v1alpha1.CompactBackup, opts v1.UpdateOptions) (result *v1alpha1.CompactBackup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(compactbackupsResource, c.ns, compactBackup), &v1alpha1.CompactBackup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.CompactBackup), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeCompactBackups) UpdateStatus(ctx context.Context, compactBackup *v1alpha1.CompactBackup, opts v1.UpdateOptions) (*v1alpha1.CompactBackup, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(compactbackupsResource, "status", c.ns, compactBackup), &v1alpha1.CompactBackup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.CompactBackup), err
}

// Delete takes name of the compactBackup and deletes it. Returns an error if one occurs.
func (c *FakeCompactBackups) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(compactbackupsResource, c.ns, name, opts), &v1alpha1.CompactBackup{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeCompactBackups) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(compactbackupsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.CompactBackupList{})
	return err
}

// Patch applies the patch and returns the patched compactBackup.
func (c *FakeCompactBackups) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.CompactBackup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(compactbackupsResource, c.ns, name, pt, data, subresources...), &v1alpha1.CompactBackup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.CompactBackup), err
}