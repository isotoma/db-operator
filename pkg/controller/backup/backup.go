package backup

import (
	"context"
	"fmt"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	util "github.com/isotoma/db-operator/pkg/util"
)


type JobConfig struct {
	Name string
	ServiceAccountName string
	Env []corev1.EnvVar
}

func (r *ReconcileBackup) createJob(instance *dbv1alpha1.Backup, provider *dbv1alpha1.Provider, jobConfig JobConfig) error {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: instance.Name + "-" + jobConfig.Name + "-",
			Namespace:    instance.Namespace,
			Labels: map[string]string{
				"app": instance.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: jobConfig.ServiceAccountName,
					Containers: []corev1.Container {
						corev1.Container{
							Name: fmt.Sprintf("%s-%s-%s", instance.Name, provider.Name, jobConfig.Name),
							Image: provider.Spec.Image,
							Command: provider.Spec.Command,
							Env: jobConfig.Env,
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}
	err := r.client.Create(context.TODO(), job)
	if err != nil {
		return err
	}
	jobName := job.ObjectMeta.Name
	log.Info(fmt.Sprintf("Created job with name %s from generate-name %s", jobName, instance.Name))
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileBackup) Backup(instance *dbv1alpha1.Backup, provider *dbv1alpha1.Provider, serviceAccountName string) error {
	err := util.PatchBackupPhase(r.client, instance, dbv1alpha1.Starting)
	if err != nil {
		return err
	}

	config := JobConfig{
		Name: "backup",
		ServiceAccountName: serviceAccountName,
		Env: []corev1.EnvVar {
			corev1.EnvVar{
				Name: "DB_OPERATOR_ACTION",
				Value: "backup",
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_NAMESPACE",
				Value: instance.Namespace,
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_DATABASE",
				Value: instance.Spec.Database,
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_BACKUP",
				Value: instance.Name,
			},
		},
	}

	return r.createJob(instance, provider, config)
}

func (r *ReconcileBackup) UpdateDatabaseStatus(instance *dbv1alpha1.Backup, database *dbv1alpha1.Database) error {
	log.Info("Updating database status after backup")
	if database.Status.Phase == dbv1alpha1.BackupInProgress {
		err := util.PatchDatabasePhase(r.client, database, dbv1alpha1.BackupCompleted)
		if err != nil {
			log.Error(err, "Error patching database to BackupCompleted")
			return err

		}
	} else if database.Status.Phase == dbv1alpha1.BackupBeforeDeleteInProgress {
		err := util.PatchDatabasePhase(r.client, database, dbv1alpha1.BackupBeforeDeleteCompleted)
		if err != nil {
			log.Error(err, "Error patching database to BackupBeforeDeleteCompleted")
			return err

		}
	} else {
		log.Info("Database not in BackupInProgress or BackupBeforeDeleteInProgress, so not updating the status.")
	}
	return nil
}
