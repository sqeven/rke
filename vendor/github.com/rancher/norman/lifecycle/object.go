package lifecycle

import (
	"github.com/rancher/norman/clientbase"
	"github.com/rancher/norman/types/slice"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	initialized = "io.cattle.lifecycle.initialized"
)

type ObjectLifecycle interface {
	Initialize(obj runtime.Object) error
	Finalize(obj runtime.Object) error
	Updated(obj runtime.Object) error
}

type objectLifecycleAdapter struct {
	name         string
	lifecycle    ObjectLifecycle
	objectClient *clientbase.ObjectClient
}

func NewObjectLifecycleAdapter(name string, lifecycle ObjectLifecycle, objectClient *clientbase.ObjectClient) func(key string, obj runtime.Object) error {
	o := objectLifecycleAdapter{
		name:         name,
		lifecycle:    lifecycle,
		objectClient: objectClient,
	}
	return o.sync
}

func (o *objectLifecycleAdapter) sync(key string, obj runtime.Object) error {
	if obj == nil {
		return nil
	}

	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	if cont, err := o.finalize(metadata, obj); err != nil || !cont {
		return err
	}

	if cont, err := o.initialize(metadata, obj); err != nil || !cont {
		return err
	}

	return o.lifecycle.Updated(obj.DeepCopyObject())
}

func (o *objectLifecycleAdapter) finalize(metadata metav1.Object, obj runtime.Object) (bool, error) {
	// Check finalize
	if metadata.GetDeletionTimestamp() == nil {
		return true, nil
	}

	if !slice.ContainsString(metadata.GetFinalizers(), o.name) {
		return false, nil
	}

	obj = obj.DeepCopyObject()
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	var finalizers []string
	for _, finalizer := range metadata.GetFinalizers() {
		if finalizer == o.name {
			continue
		}
		finalizers = append(finalizers, finalizer)
	}
	metadata.SetFinalizers(finalizers)

	if err := o.lifecycle.Finalize(obj); err != nil {
		return false, err
	}

	_, err = o.objectClient.Update(metadata.GetName(), obj)
	return false, err
}

func (o *objectLifecycleAdapter) initializeKey() string {
	return initialized + "." + o.name
}

func (o *objectLifecycleAdapter) initialize(metadata metav1.Object, obj runtime.Object) (bool, error) {
	initialized := o.initializeKey()

	if metadata.GetLabels()[initialized] == "true" {
		return true, nil
	}

	obj = obj.DeepCopyObject()
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	if metadata.GetLabels() == nil {
		metadata.SetLabels(map[string]string{})
	}

	metadata.SetFinalizers(append(metadata.GetFinalizers(), o.name))
	metadata.GetLabels()[initialized] = "true"
	if err := o.lifecycle.Initialize(obj); err != nil {
		return false, err
	}

	_, err = o.objectClient.Update(metadata.GetName(), obj)
	return false, err
}
