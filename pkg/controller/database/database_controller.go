package database

import (
	"context"
	"fmt"
	"os"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	"github.com/isotoma/db-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"k8s.io/apimachinery/pkg/types"
)

var (
	log           = logf.Log.WithName("controller_database")
	finalizerName = "database.v1alpha1.db.isotoma.com"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Database Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDatabase{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("database-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Database
	err = c.Watch(&source.Kind{Type: &dbv1alpha1.Database{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Database
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &dbv1alpha1.Database{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDatabase{}

// ReconcileDatabase reconciles a Database object
type ReconcileDatabase struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// UpdatePhase updates the phase of the database to the one requested
func (r *ReconcileDatabase) UpdatePhase(instance *dbv1alpha1.Database, phase dbv1alpha1.DatabasePhase) error {
	instance.Status.Phase = phase
	return r.client.Update(context.TODO(), instance)
}

// Reconcile reads that state of the cluster for a Database object and makes changes based on the state read
// and what is in the Database.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDatabase) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Database")

	serviceAccountName := os.Getenv("SERVICE_ACCOUNT_NAME")

	// Fetch the Database instance
	instance := &dbv1alpha1.Database{}
	if err := r.client.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Error(err, "Database not found")
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Error finding database")
		return reconcile.Result{}, err
	}
	provider := &dbv1alpha1.Provider{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: instance.Spec.Provider}, provider); err != nil {
		reqLogger.Error(err, "Error finding provider")
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("Current phase: %s", instance.Status.Phase))
	switch {
	case instance.ObjectMeta.DeletionTimestamp != nil:
		err := <- r.BackupThenDelete(instance, provider, serviceAccountName)
		return reconcile.Result{}, err
	case instance.Status.Phase == "":
		c := r.Create(instance, provider, serviceAccountName)
		if err := <-c; err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case instance.Status.Phase == dbv1alpha1.Created:
		// the driver has completed the creation process. We need to add a
		// finalizer so we have the opportunity to drop/backup the database
		// if this resource is deleted
		if util.AddFinalizer(&instance.ObjectMeta, finalizerName) {
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	case instance.Status.Phase == dbv1alpha1.DeletionRequested:
		c := r.Drop(instance, provider, serviceAccountName)
		if err := <-c; err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case instance.Status.Phase == dbv1alpha1.BackupRequested:
		c := r.Backup(instance, provider, serviceAccountName)
		if err := <-c; err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case instance.Status.Phase == dbv1alpha1.BackupBeforeDeleteRequested:
		c := r.BackupThenDelete(instance, provider, serviceAccountName)
		if err := <-c; err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case instance.Status.Phase == dbv1alpha1.BackupBeforeDeleteCompleted:
		// The driver has completed the deletion process, so we can remove
		// the finalizer and allow the resource to be finally deleted
		if util.RemoveFinalizer(&instance.ObjectMeta, finalizerName) {
			if err := r.client.Update(context.TODO(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}
