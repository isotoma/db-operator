package database

import (
	"context"
	"fmt"
	"time"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/isotoma/db-operator/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)


type JobConfig struct {
	Name string
	ServiceAccountName string
	Env []corev1.EnvVar
}

func (r *ReconcileDatabase) createJob(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, jobConfig JobConfig) error {
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

func (r *ReconcileDatabase) createBackupResource(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider) error {
	bk := &dbv1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: instance.Name + "-backup-",
			Namespace:    instance.Namespace,
			Labels: map[string]string{
				"app": instance.Name,
			},
		},
		Spec: dbv1alpha1.BackupSpec{
			Database: instance.Name,
			Serial:   time.Now().Format(time.RFC3339),
		},
		Status: dbv1alpha1.BackupStatus{},
	}
	if err := controllerutil.SetControllerReference(instance, bk, r.scheme); err != nil {
		return err
	}

	if err := r.client.Create(context.TODO(), bk); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Created backup resource %s in namespace %s", bk.ObjectMeta.Name, instance.Namespace))
	return nil
}

func (r *ReconcileDatabase) Create(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) error {
	config := JobConfig{
		Name: "create",
		ServiceAccountName: serviceAccountName,
		Env: []corev1.EnvVar {
			corev1.EnvVar{
				Name: "DB_OPERATOR_ACTION",
				Value: "create",
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_NAMESPACE",
				Value: instance.Namespace,
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_DATABASE",
				Value: instance.Name,
			},
		},
	}
	return r.createJob(instance, provider, config)
}

func (r *ReconcileDatabase) Drop(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) error {
	config := JobConfig{
		Name: "drop",
		ServiceAccountName: serviceAccountName,
		Env: []corev1.EnvVar {
			corev1.EnvVar{
				Name: "DB_OPERATOR_ACTION",
				Value: "drop",
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_NAMESPACE",
				Value: instance.Namespace,
			},
			corev1.EnvVar{
				Name: "DB_OPERATOR_DATABASE",
				Value: instance.Name,
			},
		},
	}

	return r.createJob(instance, provider, config)
}

func (r *ReconcileDatabase) Backup(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider) error {
	err := util.PatchDatabasePhase(r.client, instance, dbv1alpha1.BackupInProgress)
	if err != nil {
		return err
	}
	err = r.createBackupResource(instance, provider)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileDatabase) BackupThenDelete(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider) error {
	err := util.PatchDatabasePhase(r.client, instance, dbv1alpha1.BackupBeforeDeleteInProgress)
	if err != nil {
		return err
	}
	err = r.createBackupResource(instance, provider)
	if err != nil {
		return err
	}
	return nil
}
