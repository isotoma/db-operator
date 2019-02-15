package backup

import (
	"context"
	"fmt"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_backup")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Backup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBackup{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("backup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Backup
	err = c.Watch(&source.Kind{Type: &dbv1alpha1.Backup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Backup
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &dbv1alpha1.Backup{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileBackup{}

// ReconcileBackup reconciles a Backup object
type ReconcileBackup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Backup object and makes changes based on the state read
// and what is in the Backup.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Backup")

	// TODO: work out how to pass this in. I think this ought to be an env-var.
	serviceAccountName := "db-operator-stage"

	// Fetch the Backup instance
	instance := &dbv1alpha1.Backup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	database := &dbv1alpha1.Database{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: instance.Spec.Database}, instance); err != nil {
		reqLogger.Error(err, "Error finding database")
		return reconcile.Result{}, err
	}

	provider := &dbv1alpha1.Provider{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: database.Spec.Provider}, provider); err != nil {
		reqLogger.Error(err, "Error finding provider")
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("Current phase: %s", instance.Status.Phase))

	switch {
	case instance.Status.Phase == "":
		c := r.Backup(instance, provider, serviceAccountName)
		if err := <-c; err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileBackup) UpdatePhase(instance *dbv1alpha1.Backup, phase dbv1alpha1.BackupPhase) error {
	instance.Status.Phase = phase
	return r.client.Update(context.TODO(), instance)
}
