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
	Name     string             // name of the driver
	Connect  ConnectionDetails  // any additional connection details (eg port, host, etc)
	Master   Credentials        // credentials that allow access to create/backup/drop
	Database Credentials        // credentials to be created that have access to the database
	DBName   string             // the name of the database
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
	masterUsername, err := p.getCredential(spec.MasterCredentials.Username)
	if err != nil {
		return nil, err
	}
	masterPassword, err := p.getCredential(spec.MasterCredentials.Password)
	if err != nil {
		return nil, err
	}
	driver.Master.Username = masterUsername
	driver.Master.Password = masterPassword
	userUsername, err := p.getCredential(spec.UserCredentials.Username)
	if err != nil {
		return nil, err
	}
	userPassword, err := p.getCredential(spec.UserCredentials.Password)
	if err != nil {
		return nil, err
	}
	driver.Database.Username = userUsername
	driver.Database.Password = userPassword
	driver.DBName = spec.Name
	return driver, nil
}

func PatchDatabasePhase(k8sclient client.Client, database *dbv1alpha1.Database, phase dbv1alpha1.DatabasePhase) error {
	database.Status.Phase = phase
	log.Info(fmt.Sprintf("Patching %s to %s", database.Name, phase))
	err := k8sclient.Update(context.TODO(), database)
	if err != nil {
		log.Error(err, "Error making update")
		return err
	}
	return nil
}

func (p *Container) reconcileDatabase() error {
	phase := p.database.Status.Phase
	driver, err := p.getDriver()
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Current phase is %s", phase))

	switch {
	case phase == "":
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Creating)
		if err != nil {
			return err
		}
		err = driver.Create(driver)
		if err != nil {
			return err
		}
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Created)
		if err != nil {
			return err
		}
	case phase == dbv1alpha1.Creating:
		err = driver.Create(driver)
		if err != nil {
			return err
		}
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Created)
		if err != nil {
			return err
		}
	case phase == dbv1alpha1.Created:
		log.Info(fmt.Sprintf("Nothing to do"))
	case phase == dbv1alpha1.DeletionRequested:
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.DeletionInProgress)
		if err != nil {
			return err
		}
		err = driver.Drop(driver)
		if err != nil {
			return err
		}
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Deleted)
		if err != nil {
			return err
		}
	case phase == dbv1alpha1.DeletionInProgress:
		err = driver.Drop(driver)
		if err != nil {
			return err
		}
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Deleted)
		if err != nil {
			return err
		}
	case phase == dbv1alpha1.Deleted:
		log.Info(fmt.Sprintf("Nothing to do"))
	case phase == dbv1alpha1.BackupBeforeDeleteRequested:
		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteInProgress)
		if err != nil {
			return err
		}
		// TODO: decide how to wrangle the writer

		// err = driver.Backup(driver, writer[?])
		// if err != nil {
		// 	return err
		// }

		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteCompleted)
		if err != nil {
			return err
		}

		// TODO: should this then do the dropping too?

	case phase == dbv1alpha1.BackupBeforeDeleteInProgress:
		// TODO: more checking, or just initiate the backup-the-drop?
		// TODO: decide how to wrangle the writer

		// err = driver.Backup(driver, writer[?])
		// if err != nil {
		// 	return err
		// }

		err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteCompleted)
		if err != nil {
			return err
		}

		// TODO: should this then do the dropping too?

	case phase == dbv1alpha1.BackupBeforeDeleteCompleted:
		log.Info(fmt.Sprintf("Nothing to do"))
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
