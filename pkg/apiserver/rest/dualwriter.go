package rest

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/grafana/pkg/infra/kvstore"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

var (
	_ rest.Storage              = (DualWriter)(nil)
	_ rest.Scoper               = (DualWriter)(nil)
	_ rest.TableConvertor       = (DualWriter)(nil)
	_ rest.CreaterUpdater       = (DualWriter)(nil)
	_ rest.CollectionDeleter    = (DualWriter)(nil)
	_ rest.GracefulDeleter      = (DualWriter)(nil)
	_ rest.SingularNameProvider = (DualWriter)(nil)
)

// Storage is a storage implementation that satisfies the same interfaces as genericregistry.Store.
type Storage interface {
	rest.Storage
	rest.Scoper
	rest.TableConvertor
	rest.SingularNameProvider
	rest.Getter
	// TODO: when watch is implemented, we can replace all the below with rest.StandardStorage
	rest.Lister
	rest.CreaterUpdater
	rest.GracefulDeleter
	rest.CollectionDeleter
}

// LegacyStorage is a storage implementation that writes to the Grafana SQL database.
type LegacyStorage interface {
	rest.Storage
	rest.Scoper
	rest.SingularNameProvider
	rest.CreaterUpdater
	rest.Lister
	rest.GracefulDeleter
	rest.CollectionDeleter
	rest.TableConvertor
	rest.Getter
}

// DualWriter is a storage implementation that writes first to LegacyStorage and then to Storage.
// If writing to LegacyStorage fails, the write to Storage is skipped and the error is returned.
// Storage is used for all read operations.  This is useful as a migration step from SQL based
// legacy storage to a more standard kubernetes backed storage interface.
//
// NOTE: Only values supported by legacy storage will be preserved in the CREATE/UPDATE commands.
// For example, annotations, labels, and managed fields may not be preserved.  Everything in upstream
// storage can be recrated from the data in legacy storage.
//
// The LegacyStorage implementation must implement the following interfaces:
// - rest.Storage
// - rest.TableConvertor
// - rest.Scoper
// - rest.SingularNameProvider
//
// These interfaces are optional, but they all should be implemented to fully support dual writes:
// - rest.Creater
// - rest.Updater
// - rest.GracefulDeleter
// - rest.CollectionDeleter

type DualWriter interface {
	Storage
	LegacyStorage
	Mode() DualWriterMode
}

type DualWriterMode int

const (
	Mode1 DualWriterMode = iota + 1
	Mode2
	Mode3
	Mode4
)

// NewDualWriter returns a new DualWriter.
func NewDualWriter(mode DualWriterMode, legacy LegacyStorage, storage Storage) DualWriter {
	switch mode {
	case Mode1:
		// read and write only from legacy storage
		return NewDualWriterMode1(legacy, storage)
	case Mode2:
		// write to both, read from storage but use legacy as backup
		return NewDualWriterMode2(legacy, storage)
	case Mode3:
		// write to both, read from storage only
		return NewDualWriterMode3(legacy, storage)
	case Mode4:
		// read and write only from storage
		return NewDualWriterMode4(legacy, storage)
	default:
		return NewDualWriterMode1(legacy, storage)
	}
}

type updateWrapper struct {
	upstream rest.UpdatedObjectInfo
	updated  runtime.Object
}

// Returns preconditions built from the updated object, if applicable.
// May return nil, or a preconditions object containing nil fields,
// if no preconditions can be determined from the updated object.
func (u *updateWrapper) Preconditions() *metav1.Preconditions {
	return u.upstream.Preconditions()
}

// UpdatedObject returns the updated object, given a context and old object.
// The only time an empty oldObj should be passed in is if a "create on update" is occurring (there is no oldObj).
func (u *updateWrapper) UpdatedObject(ctx context.Context, oldObj runtime.Object) (newObj runtime.Object, err error) {
	return u.updated, nil
}

/*
TODOS
- fix kvstore get request to include orgID and namespace. potentially switch to namespacedkvstore instead.
*/

func SetDualWritingMode(
	kvs kvstore.KVStore,
	features featuremgmt.FeatureToggles,
	entity string,
	stackID string,
	legacy LegacyStorage,
	storage Storage,
) (DualWriter, error) {
	toMode := map[string]DualWriterMode{
		"1": Mode1,
		"2": Mode2,
		"3": Mode3,
		"4": Mode4,
	}
	errDualWriterSetCurrentMode := errors.New("failed to set current dual writing mode")

	key := entity + "_" + stackID
	m, ok, err := kvs.Get(context.Background(), 0, "", key)
	if err != nil {
		return nil, errors.New("failed to fetch current dual writing mode")
	}
	if !ok {
		// default to mode 1
		m = fmt.Sprint(Mode1)
		err := kvs.Set(context.Background(), 0, "", key, m)
		if err != nil {
			return nil, errDualWriterSetCurrentMode
		}
	}

	currentMode := toMode[m]

	// desired mode is 2 and current mode is 1
	if features.IsEnabledGlobally(featuremgmt.FlagDualWritePlaylistsMode2) && (currentMode == Mode1) {
		// This is where we go through the different gates to allow the instance to migrate from mode 1 to mode 2.
		// There are none between mode 1 and mode 2
		currentMode = Mode2

		err := kvs.Set(context.Background(), 0, "", key, fmt.Sprint(currentMode))
		if err != nil {
			return nil, errDualWriterSetCurrentMode
		}
	}
	// 	#TODO add support for other combinations of desired and current modes

	return NewDualWriter(currentMode, legacy, storage), nil
}
