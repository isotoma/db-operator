package database

import dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"

func (r *ReconcileDatabase) Create(instance *dbv1alpha1.Database) chan error {
	c := make(chan error)
	go func() {
		// Create a Job and wait for it
		c <- nil
	}()
	return c
}

func (r *ReconcileDatabase) Drop(instance *dbv1alpha1.Database) chan error {
	c := make(chan error)
	go func() {
		// Create a Job and wait for it
		c <- nil
	}()
	return c
}

func (r *ReconcileDatabase) BackupThenDrop(instance *dbv1alpha1.Database) chan error {
	c := make(chan error)
	go func() {
		c2 := r.Backup(instance)
		err := <-c2
		if err != nil {
			c <- err
		} else {
			// call Drop
			c <- nil
		}
	}()
	return c
}
