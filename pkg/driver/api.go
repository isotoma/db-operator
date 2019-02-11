package driver

import (
	"context"
	"fmt"
	"io"
	"os"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/isotoma/db-operator/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

type Container struct {
	k8sclient client.Client
	backup    dbv1alpha1.Backup
	database  dbv1alpha1.Database
	secret    corev1.Secret
	Namespace string
	Database  string
	Backup    string
	drivers   map[string]*Driver
}

type ConnectionDetails map[string]string

type Credentials struct {
	Username string
	Password string
}

type Driver struct {
	Name     string
	Connect  ConnectionDetails
	Master   Credentials
	Database Credentials
	Create   func(*Driver) error
	Drop     func(*Driver) error
	Backup   func(*Driver, *io.Writer) error
}

var log logr.Logger

// RegisterDriver registers your driver with the provider
func (p *Container) RegisterDriver(d *Driver) error {
	if d.Name == "" {
		return fmt.Errorf("No name provided for driver")
	}
	if p.drivers == nil {
		p.drivers = make(map[string]*Driver)
	}
	p.drivers[d.Name] = d
	return nil
}

func (p *Container) connect() error {
	log.Info("Connecting to Kubernetes")
	cfg := config.GetConfigOrDie()
	managerOptions := manager.Options{}
	mgr, err := manager.New(cfg, managerOptions)
	if err != nil {
		return err
	}
	log.Info("Adding APIs to scheme")
	err = apis.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
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
	p.k8sclient = mgr.GetClient()
	return nil
}

func (p *Container) getResource(name string, dest runtime.Object) error {
	return p.k8sclient.Get(context.TODO(), types.NamespacedName{Namespace: p.Namespace, Name: name}, dest)
}

func (p *Container) setup() error {
	// This will be populated using the downward API
	if p.Namespace == "" {
		p.Namespace = os.Getenv("DB_OPERATOR_NAMESPACE")
	}
	if p.Namespace == "" {
		return fmt.Errorf("Namespace not specified")
	}
	if p.Database == "" {
		p.Database = os.Getenv("DB_OPERATOR_DATABASE")
	}
	if p.Backup == "" {
		p.Backup = os.Getenv("DB_OPERATOR_BACKUP")
	}
	if p.Database == "" && p.Backup == "" {
		return fmt.Errorf("No database or backup name provided")
	}
	if err := p.connect(); err != nil {
		return err
	}
	if p.Database != "" {
		if err := p.getResource(p.Database, &p.database); err != nil {
			return err
		}
		if err := p.getResource(p.Database, &p.secret); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}
	if p.Backup != "" {
		if err := p.getResource(p.Backup, &p.backup); err != nil {
			return err
		}
	}
	return nil
}

func (p *Container) readFromKubernetesSecret(s dbv1alpha1.SecretKeyRef) (string, error) {
	return "", nil
}

func (p *Container) readFromAwsSecret(s dbv1alpha1.AwsSecretRef) (string, error) {
	return "", nil
}

func (p *Container) getCredential(cred dbv1alpha1.Credential) (string, error) {
	if cred.Value != "" {
		return cred.Value, nil
	}
	if cred.ValueFrom.SecretKeyRef.Name != "" {
		return p.readFromKubernetesSecret(cred.ValueFrom.SecretKeyRef)
	}
	if cred.ValueFrom.AwsSecretKeyRef.ARN != "" {
		return p.readFromAwsSecret(cred.ValueFrom.AwsSecretKeyRef)
	}
	return "", fmt.Errorf("No credentials provided")
}

func (p *Container) getDriver() (*Driver, error) {
	spec := p.database.Spec
	driver := p.drivers[spec.Provider]
	driver.Connect = spec.Connect
	username, err := p.getCredential(spec.Credentials.Username)
	if err != nil {
		return nil, err
	}
	password, err := p.getCredential(spec.Credentials.Password)
	if err != nil {
		return nil, err
	}
	driver.Master.Username = username
	driver.Master.Password = password
	driver.Database.Username = spec.Name
	return driver, nil
}

func (p *Container) reconcileDatabase() error {
	phase := p.database.Status.Phase
	driver, err := p.getDriver()
	if err != nil {
		return err
	}

	switch {
	case phase == "":
		err = driver.Create(driver)
		// change state to creating and call Create
		// if it terminates without error then move state to Created
	case phase == dbv1alpha1.Creating:
		// We've been terminated during a creation
		// call Create again
		// if it terminates without error then move state to Created
	case phase == dbv1alpha1.Created:
		// We don't need to do anything
	case phase == dbv1alpha1.DeletionRequested:
		// do the delete
	case phase == dbv1alpha1.DeletionInProgress:
		// check status
	case phase == dbv1alpha1.Deleted:
		// do nothing
	case phase == dbv1alpha1.BackupBeforeDeleteRequested:
		// perform the backup
	case phase == dbv1alpha1.BackupBeforeDeleteInProgress:
		// check status
	case phase == dbv1alpha1.BackupBeforeDeleteCompleted:
		// do nothing

	}
	return nil
}

func (p *Container) reconcileBackup() error {
	return nil
}

// Run the provider, which will reconcile the provided database/backup
// using the registered drivers
func (p *Container) Run() error {
	zlog, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	log = zapr.NewLogger(zlog).WithName("db-operator")

	if err := p.setup(); err != nil {
		return err
	}
	if p.Database != "" {
		return p.reconcileDatabase()
	}
	if p.Backup != "" {
		return p.reconcileBackup()
	}
	return nil
}
