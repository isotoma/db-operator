package database

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Create a Backup resource for the database and return its name
func (r *ReconcileDatabase) createBackupResource(instance *dbv1alpha1.Database) (*dbv1alpha1.Backup, error) {
	backup := &dbv1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: instance.Name + "-",
			Namespace:    instance.Namespace,
			Labels: map[string]string{
				"app": instance.Name,
			},
		},
		Spec: dbv1alpha1.BackupSpec{
			Database: instance.Name,
			Serial:   time.Now().Format(time.RFC3339),
		},
	}
	if err := controllerutil.SetControllerReference(instance, backup, r.scheme); err != nil {
		return nil, err
	}
	if err := r.client.Create(context.TODO(), backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func (r *ReconcileDatabase) blockUntilCompleted(Namespace, Name string) error {
	delay, _ := time.ParseDuration("30s")
	for {
		time.Sleep(delay)
		found := dbv1alpha1.Backup{}
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: Namespace,
			Name:      Name,
		}, &found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Waiting for Backup resource")
		} else if err != nil {
			return err
		} else {
			if found.Status.Phase == dbv1alpha1.Completed {
				return nil
			}
			log.Info(fmt.Sprintf("Backup phase is %s, waiting", found.Status.Phase))
		}
	}
}

// Backup creates a backup resource and returns a channel that will
// send nil/an error when the backup is completed
func (r *ReconcileDatabase) Backup(instance *dbv1alpha1.Database) chan error {
	c := make(chan error)
	go func() {
		backup, err := r.createBackupResource(instance)
		if err != nil {
			c <- err
			return
		}
		c <- r.blockUntilCompleted(backup.Namespace, backup.Name)
	}()
	return c
}
