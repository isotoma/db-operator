package backup

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

	util "github.com/isotoma/db-operator/pkg/util"
)


type JobConfig struct {
	Name string
	ServiceAccountName string
	Env []corev1.EnvVar
}

func (r *ReconcileBackup) blockUntilJobCompleted(Namespace, Name string) error {
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

func (r *ReconcileBackup) createJobAndBlock(instance *dbv1alpha1.Backup, provider *dbv1alpha1.Provider, jobConfig JobConfig) error {
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

func (r *ReconcileBackup) Backup(instance *dbv1alpha1.Backup, provider *dbv1alpha1.Provider, serviceAccountName string) chan error {
	c := make(chan error)
	go func() {
		err := util.PatchBackupPhase(r.client, instance, dbv1alpha1.Starting)
		if err != nil {
			c <- err
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

		err = r.createJobAndBlock(instance, provider, config)

		if err != nil {
			c <- err
		}

		c <- nil
	}()
	return c
}
