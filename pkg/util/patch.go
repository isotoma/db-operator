package util

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
)

var log = logf.Log.WithName("util/patch")

func PatchDatabasePhase(k8sclient client.Client, database *dbv1alpha1.Database, phase dbv1alpha1.DatabasePhase) error {
	for {
		database.Status.Phase = phase
		log.Info(fmt.Sprintf("Patching %s to %s", database.Name, phase))
		err := k8sclient.Status().Update(context.TODO(), database)
		if err != nil {
			if errors.IsConflict(err) {
				log.Info("Encountered conflict error, retrying")
				// TODO: backoff
				err = RefreshDatabase(k8sclient, database)
				if err != nil {
					return err
				}
				continue
			} else {
				log.Error(err, "Error updating database status")
				return err
			}
		}
		break
	}
	return nil
}

func RefreshDatabase(k8sclient client.Client, database *dbv1alpha1.Database) error {
	return k8sclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: database.ObjectMeta.Namespace,
			Name: database.ObjectMeta.Name,
		},
		database)
}

func PatchBackupPhase(k8sclient client.Client, backup *dbv1alpha1.Backup, phase dbv1alpha1.BackupPhase) error {
	for {
		backup.Status.Phase = phase
		log.Info(fmt.Sprintf("Patching %s to %s", backup.Name, phase))
		err := k8sclient.Status().Update(context.TODO(), backup)
		if err != nil {
			if errors.IsConflict(err) {
				log.Info("Encountered conflict error, retrying")
				// TODO: backoff
				err = RefreshBackup(k8sclient, backup)
				if err != nil {
					return err
				}
				continue
			} else {
				log.Error(err, "Error updating backup status")
				return err
			}
			return err
		}
		break
	}
	return nil
}

func RefreshBackup(k8sclient client.Client, backup *dbv1alpha1.Backup) error {
	return k8sclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: backup.ObjectMeta.Namespace,
			Name: backup.ObjectMeta.Name,
		},
		backup)
}
