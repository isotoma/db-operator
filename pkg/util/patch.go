package util

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
)

var log = logf.Log.WithName("util/patch")

func PatchDatabasePhase(k8sclient client.Client, database *dbv1alpha1.Database, phase dbv1alpha1.DatabasePhase) error {
	database.Status.Phase = phase
	log.Info(fmt.Sprintf("Patching %s to %s", database.Name, phase))
	err := k8sclient.Status().Update(context.TODO(), database)
	if err != nil {
		log.Error(err, "Error updating database status")
		return err
	}
	return nil
}

func PatchBackupPhase(k8sclient client.Client, backup *dbv1alpha1.Backup, phase dbv1alpha1.BackupPhase) error {
	backup.Status.Phase = phase
	log.Info(fmt.Sprintf("Patching %s to %s", backup.Name, phase))
	err := k8sclient.Status().Update(context.TODO(), backup)
	if err != nil {
		err := k8sclient.Update(context.TODO(), backup)
		log.Error(err, "Error updating backup status (maybe the feature flag is disabled)")
		log.Info("Trying to just update the whole resource")
		if err != nil {
			log.Error(err, "Error making update")
			return err
		}
	}
	return nil
}
