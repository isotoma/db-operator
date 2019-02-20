package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/apimachinery/pkg/types"
)


type JobConfig struct {
	Name string
	ServiceAccountName string
	Env []corev1.EnvVar
}

func (r *ReconcileDatabase) blockUntilJobCompleted(Namespace, Name string) error {
	delay, _ := time.ParseDuration("30s")
	for {
		time.Sleep(delay)
		found := batchv1.Job{}
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: Namespace,
			Name:      Name,
		}, &found)
		if err != nil && k8serrors.IsNotFound(err) {
			log.Info("Waiting for Job resource")
		} else if err != nil {
			return err
		} else {
			if found.Status.Succeeded > 0 {
				return nil
			} else if found.Status.Failed > 0 {
				return errors.New("Job failed")
			} else {
				log.Info(fmt.Sprintf("Job phase is %+v, waiting", found.Status))
			}
		}
	}
}

func (r *ReconcileDatabase) createJobAndBlock(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, jobConfig JobConfig) error {
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

	log.Info(fmt.Sprintf("Waiting for job %s in namespace %s to complete", jobName, instance.Namespace))
	err = r.blockUntilJobCompleted(instance.Namespace, jobName)
	if err != nil {
		log.Info(fmt.Sprintf("Error while waiting for job %s in namespace %s to complete", jobName, instance.Namespace))
		return err
	}

	log.Info(fmt.Sprintf("Job %s in namespace %s completed", jobName, instance.Namespace))

	return nil
}

func (r *ReconcileDatabase) blockUntilBackupCompleted(Namespace, Name string) error {
	delay, _ := time.ParseDuration("30s")
	for {
		time.Sleep(delay)
		found := dbv1alpha1.Backup{}
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: Namespace,
			Name:      Name,
		}, &found)
		if err != nil && k8serrors.IsNotFound(err) {
			log.Info("Waiting for backup resource")
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

func (r *ReconcileDatabase) createBackupResourceAndBlock(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider) error {
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

	backupName := bk.ObjectMeta.Name
	log.Info(fmt.Sprintf("Waiting for backup %s in namespace %s to complete", backupName, instance.Namespace))
	if err := r.blockUntilBackupCompleted(instance.Namespace, backupName); err != nil {
		log.Info(fmt.Sprintf("Error while waiting for backup %s in namespace %s to complete", backupName, instance.Namespace))
		return err
	}

	log.Info(fmt.Sprintf("Backup %s in namespace %s completed", backupName, instance.Namespace))
	return nil
}

func (r *ReconcileDatabase) Create(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
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

		c <- r.createJobAndBlock(instance, provider, config)
	}()
	return c
}

func (r *ReconcileDatabase) Drop(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
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

		c <- r.createJobAndBlock(instance, provider, config)
	}()
	return c
}

func (r *ReconcileDatabase) Backup(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		err := r.UpdatePhase(instance, dbv1alpha1.BackupInProgress)
		if err != nil {
			c <- err
		}
		err = r.createBackupResourceAndBlock(instance, provider)
		if err != nil {
			c <- err
		}
		c <- nil
	}()
	return c
}

func (r *ReconcileDatabase) BackupThenDelete(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		err := r.UpdatePhase(instance, dbv1alpha1.BackupBeforeDeleteInProgress)
		if err != nil {
			c <- err
		}
		err = r.createBackupResourceAndBlock(instance, provider)
		if err != nil {
			c <- err
		}
		err = r.UpdatePhase(instance, dbv1alpha1.BackupBeforeDeleteCompleted)
		if err != nil {
			c <- err
		}
		c <- nil
	}()
	return c
}
