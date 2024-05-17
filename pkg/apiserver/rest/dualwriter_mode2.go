package rest

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/grafana/grafana/pkg/services/apiserver/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/v2"
)

type DualWriterMode2 struct {
	Storage Storage
	Legacy  LegacyStorage
	Log     klog.Logger
	*dualWriterMetrics
}

var mode2 = strconv.Itoa(int(Mode2))

// NewDualWriterMode2 returns a new DualWriter in mode 2.
// Mode 2 represents writing to LegacyStorage and Storage and reading from LegacyStorage.
func NewDualWriterMode2(legacy LegacyStorage, storage Storage) *DualWriterMode2 {
	metrics := &dualWriterMetrics{}
	metrics.init()
	return &DualWriterMode2{Legacy: legacy, Storage: storage, Log: klog.NewKlogr().WithName("DualWriterMode2"), dualWriterMetrics: metrics}
}

// Create overrides the behavior of the generic DualWriter and writes to LegacyStorage and Storage.
func (d *DualWriterMode2) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	log := d.Log.WithValues("kind", options.Kind)
	ctx = klog.NewContext(ctx, log)
	var method = "create"

	startLegacy := time.Now().UTC()
	created, err := d.Legacy.Create(ctx, obj, createValidation, options)
	if err != nil {
		log.Error(err, "unable to create object in legacy storage")
		d.recordLegacyDuration(true, mode2, options.Kind, method, startLegacy)
		return created, err
	}
	d.recordLegacyDuration(false, mode2, options.Kind, method, startLegacy)

	accessorCreated, err := meta.Accessor(created)
	if err != nil {
		return created, err
	}

	accessorOld, err := meta.Accessor(obj)
	if err != nil {
		return created, err
	}

	enrichObject(accessorOld, accessorCreated)

	// create method expects an empty resource version
	accessorCreated.SetResourceVersion("")
	accessorCreated.SetUID("")

	startStorage := time.Now().UTC()
	rsp, err := d.Storage.Create(ctx, created, createValidation, options)
	if err != nil {
		log.WithValues("name", accessorCreated.GetName(), "resourceVersion", accessorCreated.GetResourceVersion()).Error(err, "unable to create object in storage")
	}
	d.recordStorageDuration(err != nil, mode2, options.Kind, method, startStorage)
	return rsp, err
}

// Get overrides the behavior of the generic DualWriter.
// It returns legacy object as source of truth. Same as mode1
func (d *DualWriterMode2) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	log := d.Log.WithValues("name", name, "resourceVersion", options.ResourceVersion, "kind", options.Kind)
	ctx = klog.NewContext(ctx, log)
	var method = "get"

	startLegacy := time.Now().UTC()
	res, err := d.Legacy.Get(ctx, name, options)
	if err != nil {
		log.Error(err, "unable to get object in legacy storage")
		d.recordLegacyDuration(true, mode2, name, method, startLegacy)
		return res, err
	}
	d.recordLegacyDuration(false, mode2, name, method, startLegacy)

	go func() {
		startStorage := time.Now().UTC()
		ctx, _ := context.WithTimeoutCause(ctx, time.Second*10, errors.New("storage get timeout"))
		_, err := d.Storage.Get(ctx, name, options)
		defer d.recordStorageDuration(err != nil, mode2, name, method, startStorage)
	}()

	return res, err
}

// List overrides the behavior of the generic DualWriter.
// It returns Storage entries if possible and falls back to LegacyStorage entries if not.
func (d *DualWriterMode2) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	log := d.Log.WithValues("kind", options.Kind, "resourceVersion", options.ResourceVersion, "kind", options.Kind)
	ctx = klog.NewContext(ctx, log)
	var method = "list"

	startLegacy := time.Now().UTC()
	ll, err := d.Legacy.List(ctx, options)
	if err != nil {
		log.Error(err, "unable to list objects from legacy storage")
	}
	d.recordLegacyDuration(err != nil, mode2, options.Kind, method, startLegacy)

	legacyList, err := meta.ExtractList(ll)
	if err != nil {
		log.Error(err, "unable to extract list from legacy storage")
		return nil, err
	}

	// Record the index of each LegacyStorage object so it can later be replaced by
	// an equivalent Storage object if it exists.
	optionsStorage, indexMap, err := parseList(legacyList)
	if err != nil {
		return nil, err
	}

	// TODO: why do we need this?
	if optionsStorage.LabelSelector == nil {
		return ll, nil
	}

	startStorage := time.Now().UTC()
	sl, err := d.Storage.List(ctx, &optionsStorage)
	if err != nil {
		log.Error(err, "unable to list objects from storage")
	}
	d.recordStorageDuration(err != nil, mode2, options.Kind, method, startStorage)

	storageList, err := meta.ExtractList(sl)
	if err != nil {
		log.Error(err, "unable to extract list from storage")
		return nil, err
	}

	for _, obj := range storageList {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		if legacyIndex, ok := indexMap[accessor.GetName()]; ok {
			legacyList[legacyIndex] = obj
		}
	}

	if err = meta.SetList(ll, legacyList); err != nil {
		return nil, err
	}
	return ll, nil
}

// DeleteCollection overrides the behavior of the generic DualWriter and deletes from both LegacyStorage and Storage.
func (d *DualWriterMode2) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	log := d.Log.WithValues("kind", options.Kind, "resourceVersion", listOptions.ResourceVersion)
	ctx = klog.NewContext(ctx, log)
	var method = "delete-collection"

	startLegacy := time.Now().UTC()
	deleted, err := d.Legacy.DeleteCollection(ctx, deleteValidation, options, listOptions)
	if err != nil {
		log.WithValues("deleted", deleted).Error(err, "failed to delete collection successfully from legacy storage")
	}
	d.recordLegacyDuration(err != nil, mode2, options.Kind, method, startLegacy)

	legacyList, err := meta.ExtractList(deleted)
	if err != nil {
		log.Error(err, "unable to extract list from legacy storage")
		return nil, err
	}

	// Only the items deleted by the legacy DeleteCollection call are selected for deletion by Storage.
	optionsStorage, _, err := parseList(legacyList)
	if err != nil {
		return nil, err
	}
	if optionsStorage.LabelSelector == nil {
		return deleted, nil
	}

	res, err := d.Storage.DeleteCollection(ctx, deleteValidation, options, &optionsStorage)
	if err != nil {
		log.WithValues("deleted", res).Error(err, "failed to delete collection successfully from Storage")
	}

	return res, err
}

func (d *DualWriterMode2) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	log := d.Log.WithValues("name", name, "kind", options.Kind)
	ctx = klog.NewContext(ctx, log)

	deletedLS, async, err := d.Legacy.Delete(ctx, name, deleteValidation, options)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.WithValues("objectList", deletedLS).Error(err, "could not delete from legacy store")
			return deletedLS, async, err
		}
	}

	deletedS, _, errUS := d.Storage.Delete(ctx, name, deleteValidation, options)
	if errUS != nil {
		if !apierrors.IsNotFound(errUS) {
			log.WithValues("objectList", deletedS).Error(errUS, "could not delete from duplicate storage")
		}
	}

	return deletedLS, async, err
}

// Update overrides the generic behavior of the Storage and writes first to the legacy storage and then to storage.
func (d *DualWriterMode2) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	log := d.Log.WithValues("name", name, "kind", options.Kind)
	ctx = klog.NewContext(ctx, log)

	// get foundObj and new (updated) object so they can be stored in legacy store
	foundObj, err := d.Storage.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.WithValues("object", foundObj).Error(err, "could not get object to update")
			return nil, false, err
		}
		log.Info("object not found for update, creating one")
	}

	// obj can be populated in case it's found or empty in case it's not found
	updated, err := objInfo.UpdatedObject(ctx, foundObj)
	if err != nil {
		log.WithValues("object", updated).Error(err, "could not update or create object")
		return nil, false, err
	}

	obj, created, err := d.Legacy.Update(ctx, name, &updateWrapper{upstream: objInfo, updated: updated}, createValidation, updateValidation, forceAllowCreate, options)
	if err != nil {
		log.WithValues("object", obj).Error(err, "could not update in legacy storage")
		return obj, created, err
	}

	// if the object is found, create a new updateWrapper with the object found
	if foundObj != nil {
		accessorOld, err := meta.Accessor(foundObj)
		if err != nil {
			log.Error(err, "unable to get accessor for original updated object")
		}

		accessor, err := meta.Accessor(obj)
		if err != nil {
			log.Error(err, "unable to get accessor for updated object")
		}

		enrichObject(accessorOld, accessor)

		accessor.SetResourceVersion(accessorOld.GetResourceVersion())
		accessor.SetUID(accessorOld.GetUID())

		objInfo = &updateWrapper{
			upstream: objInfo,
			updated:  obj,
		}
	}
	// TODO: relies on GuaranteedUpdate creating the object if
	// it doesn't exist: https://github.com/grafana/grafana/pull/85206
	return d.Storage.Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options)
}

func (d *DualWriterMode2) Destroy() {
	d.Storage.Destroy()
	d.Legacy.Destroy()
}

func (d *DualWriterMode2) GetSingularName() string {
	return d.Storage.GetSingularName()
}

func (d *DualWriterMode2) NamespaceScoped() bool {
	return d.Storage.NamespaceScoped()
}

func (d *DualWriterMode2) New() runtime.Object {
	return d.Storage.New()
}

func (d *DualWriterMode2) NewList() runtime.Object {
	return d.Storage.NewList()
}

func (d *DualWriterMode2) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return d.Storage.ConvertToTable(ctx, object, tableOptions)
}

func parseList(legacyList []runtime.Object) (metainternalversion.ListOptions, map[string]int, error) {
	options := metainternalversion.ListOptions{}
	originKeys := []string{}
	indexMap := map[string]int{}

	for i, obj := range legacyList {
		metaAccessor, err := utils.MetaAccessor(obj)
		if err != nil {
			return options, nil, err
		}
		originKeys = append(originKeys, metaAccessor.GetOriginKey())

		accessor, err := meta.Accessor(obj)
		if err != nil {
			return options, nil, err
		}
		indexMap[accessor.GetName()] = i
	}

	if len(originKeys) == 0 {
		return options, nil, nil
	}

	r, err := labels.NewRequirement(utils.AnnoKeyOriginKey, selection.In, originKeys)
	if err != nil {
		return options, nil, err
	}
	options.LabelSelector = labels.NewSelector().Add(*r)

	return options, indexMap, nil
}

func enrichObject(accessorO, accessorC metav1.Object) {
	accessorC.SetLabels(accessorO.GetLabels())

	ac := accessorC.GetAnnotations()
	if ac == nil {
		ac = map[string]string{}
	}
	for k, v := range accessorO.GetAnnotations() {
		ac[k] = v
	}
	accessorC.SetAnnotations(ac)
}
