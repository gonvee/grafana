package rest

import (
	"context"
	"strconv"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
)

type DualWriterMode1 struct {
	Legacy  LegacyStorage
	Storage Storage
	Log     klog.Logger
	*dualWriterMetrics
}

var mode = strconv.Itoa(int(Mode1))

// NewDualWriterMode1 returns a new DualWriter in mode 1.
// Mode 1 represents writing to and reading from LegacyStorage.
func NewDualWriterMode1(legacy LegacyStorage, storage Storage) *DualWriterMode1 {
	metrics := &dualWriterMetrics{}
	metrics.init()
	return &DualWriterMode1{Legacy: legacy, Storage: storage, Log: klog.NewKlogr().WithName("DualWriterMode1"), dualWriterMetrics: metrics}
}

// Create overrides the behavior of the generic DualWriter and writes only to LegacyStorage.
func (d *DualWriterMode1) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "create"

	startStorage := time.Now().UTC()
	objStorage, err := d.Storage.Create(ctx, obj, createValidation, options)
	d.recordStorageDuration(err != nil, mode, options.Kind, method, startStorage)

	startLegacy := time.Now().UTC()
	res, err := d.Legacy.Create(ctx, obj, createValidation, options)
	if err != nil {
		klog.Error(err, "unable to create object in legacy storage")
		d.recordLegacyDuration(true, mode, options.Kind, method, startLegacy)
		return res, err
	}
	d.recordLegacyDuration(false, mode, options.Kind, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, options.Kind, areSame, method)
	return res, err
}

// Get overrides the behavior of the generic DualWriter and reads only from LegacyStorage.
func (d *DualWriterMode1) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "get"

	startStorage := time.Now().UTC()
	objStorage, err := d.Storage.Get(ctx, name, options)
	d.recordStorageDuration(err != nil, mode, name, method, startStorage)

	startLegacy := time.Now().UTC()
	res, err := d.Legacy.Get(ctx, name, options)
	if err != nil {
		klog.Error(err, "unable to get object in legacy storage")
		d.recordLegacyDuration(true, mode, name, method, startLegacy)
		return res, err
	}
	d.recordLegacyDuration(false, mode, name, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, name, areSame, method)
	return res, err
}

// List overrides the behavior of the generic DualWriter and reads only from LegacyStorage.
func (d *DualWriterMode1) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "list"

	startStorage := time.Now().UTC()
	objStorage, err := d.Storage.List(ctx, options)
	d.recordStorageDuration(err != nil, mode, options.Kind, method, startStorage)

	startLegacy := time.Now().UTC()
	res, err := d.Legacy.List(ctx, options)
	if err != nil {
		klog.Error(err, "unable to list object in legacy storage")
		d.recordLegacyDuration(true, mode, options.Kind, method, startLegacy)
		return res, err
	}
	d.recordLegacyDuration(false, mode, options.Kind, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, options.Kind, areSame, method)
	return res, err
}

func (d *DualWriterMode1) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "delete"

	startStorage := time.Now().UTC()
	objStorage, _, err := d.Storage.Delete(ctx, name, deleteValidation, options)
	d.recordStorageDuration(err != nil, mode, name, method, startStorage)

	startLegacy := time.Now().UTC()
	res, async, err := d.Legacy.Delete(ctx, name, deleteValidation, options)
	if err != nil {
		klog.Error(err, "unable to delete object in legacy storage")
		d.recordLegacyDuration(true, mode, name, method, startLegacy)
		return res, async, err
	}
	d.recordLegacyDuration(false, mode, name, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, name, areSame, method)
	return res, async, err
}

// DeleteCollection overrides the behavior of the generic DualWriter and deletes only from LegacyStorage.
func (d *DualWriterMode1) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "delete-collection"

	startStorage := time.Now().UTC()
	objStorage, err := d.Storage.DeleteCollection(ctx, deleteValidation, options, listOptions)
	d.recordStorageDuration(err != nil, mode, options.Kind, method, startStorage)

	startLegacy := time.Now().UTC()
	res, err := d.Legacy.DeleteCollection(ctx, deleteValidation, options, listOptions)
	if err != nil {
		klog.Error(err, "unable to delete collection in legacy storage")
		d.recordLegacyDuration(true, mode, options.Kind, method, startLegacy)
	}
	d.recordLegacyDuration(false, mode, options.Kind, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, options.Kind, areSame, method)
	return res, err
}

func (d *DualWriterMode1) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	ctx = klog.NewContext(ctx, d.Log)
	var method = "update"

	startStorage := time.Now().UTC()
	objStorage, _, err := d.Storage.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
	d.recordStorageDuration(err != nil, mode, name, method, startStorage)

	startLegacy := time.Now().UTC()
	res, async, err := d.Legacy.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
	if err != nil {
		klog.Error(err, "unable to update collection in legacy storage")
		d.recordLegacyDuration(true, mode, name, method, startLegacy)
	}
	d.recordLegacyDuration(false, mode, name, method, startLegacy)

	areSame, err := compareResourceVersion(objStorage, res)
	if err != nil {
		// only log error but don't return error so we keep the same behavior as before
		klog.Error(err, "unable to compare resource versions")
	}

	d.recordOutcome(mode, name, areSame, method)
	return res, async, err
}

func (d *DualWriterMode1) Destroy() {
	d.Storage.Destroy()
	d.Legacy.Destroy()
}

func (d *DualWriterMode1) GetSingularName() string {
	return d.Legacy.GetSingularName()
}

func (d *DualWriterMode1) NamespaceScoped() bool {
	return d.Legacy.NamespaceScoped()
}

func (d *DualWriterMode1) New() runtime.Object {
	return d.Legacy.New()
}

func (d *DualWriterMode1) NewList() runtime.Object {
	return d.Storage.NewList()
}

func (d *DualWriterMode1) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return d.Legacy.ConvertToTable(ctx, object, tableOptions)
}
