package database

import (
	"testing"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func fakeReconciler(objs []runtime.Object) *ReconcileDatabase {
	s := scheme.Scheme
	s.AddKnownTypes(dbv1alpha1.SchemeGroupVersion, &dbv1alpha1.Database{}, &dbv1alpha1.Backup{})
	cl := fake.NewFakeClient(objs...)
	return &ReconcileDatabase{client: cl, scheme: s}
}

func TestCreateBackupResource(t *testing.T) {
	db := &dbv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testdb",
		},
	}
	r := fakeReconciler([]runtime.Object{db})
	backup, err := r.createBackupResource(db)
	backup.Name = "testName" // fake client doesn't generate name
	if err != nil {
		t.Errorf("createBackupResource threw unexpected error: %s", err)
	}
	if backup.Status.Phase != "" {
		t.Errorf("Error in initial phase")
	}
}
