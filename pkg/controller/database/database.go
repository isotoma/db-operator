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
	"k8s.io/apimachinery/pkg/types"
)


func (r *ReconcileDatabase) createJob(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string, env []corev1.EnvVar, suffix string) (error, string) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: instance.Name + "-",
			Namespace:    instance.Namespace,
			Labels: map[string]string{
				"app": instance.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container {
						corev1.Container{
							Name: fmt.Sprintf("%s-%s-%s", instance.Name, provider.Name, suffix),
							Image: provider.Spec.Image,
							Command: provider.Spec.Command,
							Args: provider.Spec.Args,
							Env: env,
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}
	err := r.client.Create(context.TODO(), job)
	if err != nil {
		return err, ""
	}
	name := job.ObjectMeta.Name
	return nil, name
}


func (r *ReconcileDatabase) createDatabaseJob(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) (error, string) {
	env := []corev1.EnvVar {
		corev1.EnvVar{
			Name: "DB_OPERATOR_NAMESPACE",
			Value: instance.Namespace,
		},
		corev1.EnvVar{
			Name: "DB_OPERATOR_DATABASE",
			Value: instance.Name,
		},
	}
	return r.createJob(instance, provider, serviceAccountName, env, "database")
}

func (r *ReconcileDatabase) createBackupJob(instance *dbv1alpha1.Database, backup *dbv1alpha1.Backup, provider *dbv1alpha1.Provider, serviceAccountName string) (error, string) {
	env := []corev1.EnvVar {
		corev1.EnvVar{
			Name: "DB_OPERATOR_NAMESPACE",
			Value: instance.Namespace,
		},
		corev1.EnvVar{
			Name: "DB_OPERATOR_BACKUP",
			Value: backup.Name,
		},
	}
	return r.createJob(instance, provider, serviceAccountName, env, "database")
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

func (r *ReconcileDatabase) Create(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		err, jobName := r.createDatabaseJob(instance, provider, serviceAccountName)
		if err != nil {
			c <- err
			return
		}

		err = r.blockUntilJobCompleted(instance.Namespace, jobName)
		if err != nil {
			c <- err
			return
		}

		c <- nil
	}()
	return c
}

func (r *ReconcileDatabase) Drop(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		err, jobName := r.createDatabaseJob(instance, provider, serviceAccountName)
		if err != nil {
			c <- err
			return
		}

		err = r.blockUntilJobCompleted(instance.Namespace, jobName)
		if err != nil {
			c <- err
			return
		}

		c <- nil
	}()
	return c
}

func (r *ReconcileDatabase) BackupThenDrop(instance *dbv1alpha1.Database, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		c2 := r.Backup(instance)
		err := <-c2
		if err != nil {
			c <- err
			return
		}
			
		err, jobName := r.createDatabaseJob(instance, provider, serviceAccountName)
		if err != nil {
			c <- err
			return
		}

		err = r.blockUntilJobCompleted(instance.Namespace, jobName)
		if err != nil {
			c <- err
			return
		}

		c <- nil
	}()
	return c
}
