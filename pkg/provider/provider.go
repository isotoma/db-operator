package provider

import (
	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	"github.com/isotoma/db-operator/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type Provider struct {
	k8sclient    client.Client
	backup       *dbv1alpha1.Backup
	database     *dbv1alpha1.Database
	secret       *corev1.Secret
	namespace    string
	databaseName string
	backupName   string
}

var log = logf.Log.WithName("db-operator")

func initialise() (*Database, *Backup, error) {
	cfg := config.GetConfigOrDie()
	managerOptions := manager.Options{}
	mgr, err := manager.New(cfg, managerOptions)
	if err != nil {
		return nil, nil, err
	}
	log.Info("Adding APIs to scheme")
	err = apis.AddToScheme(mgr.GetScheme())
	if err != nil {
		return nil, nil, err
	}
	log.Info("Starting cache")
	var stopChan <-chan struct{}
	go func() {
		err := mgr.GetCache().Start(stopChan)
		if err != nil {
			panic(err)
		}
	}()
	mgr.GetCache().WaitForCacheSync(stopChan)
	log.Info("Getting client")
	k8sClient := mgr.GetClient()
	return nil, nil, nil

}
