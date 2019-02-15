package driver

import (
	"context"
	"fmt"
	"io"
	"os"
	"errors"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/isotoma/db-operator/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type Container struct {
	k8sclient client.Client
	backup    dbv1alpha1.Backup
	database  dbv1alpha1.Database
	secret    corev1.Secret
	Namespace string
	Database  string
	Backup    string
	Action    string
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
	Backup   func(*Driver, *io.PipeWriter) error
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
	log.Info("Got client")
	return nil
}

func (p *Container) getResource(name string, dest runtime.Object) error {
	return p.k8sclient.Get(context.TODO(), types.NamespacedName{Namespace: p.Namespace, Name: name}, dest)
}

func (p *Container) setup() error {
	// This will be populated using the downward API
	p.Namespace = os.Getenv("DB_OPERATOR_NAMESPACE")
	if p.Namespace == "" {
		return fmt.Errorf("No namespace (DB_OPERATOR_NAMESPACE) provided")
	}
	p.Database = os.Getenv("DB_OPERATOR_DATABASE")
	p.Backup = os.Getenv("DB_OPERATOR_BACKUP")
	if p.Database == "" {
		return fmt.Errorf("No database name (DB_OPERATOR_BACKUP) provided")
	}
	if p.Action == "" {
		p.Action = os.Getenv("DB_OPERATOR_ACTION")
	}
	if p.Action == "" {
		return fmt.Errorf("No action (DB_OPERATOR_ACTION) provider")
	}
	if err := p.connect(); err != nil {
		return err
	}
	log.Info("Connected")
	if p.Database != "" {
		log.Info("Getting database")
		if err := p.getResource(p.Database, &p.database); err != nil {
			return err
		}
		if err := p.getResource(p.Database, &p.secret); err != nil {
			if !k8serrors.IsNotFound(err) {
				return err
			}
		}
	}
	if p.Backup != "" {
		log.Info("Getting backup")
		if err := p.getResource(p.Backup, &p.backup); err != nil {
			return err
		}
		if err := p.getResource(p.backup.Spec.Database, &p.database); err != nil {
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
	log.Info("Getting spec for database")
	spec := p.database.Spec
	log.Info(fmt.Sprintf("Getting driver for provider %s", spec.Provider))
	driver := p.drivers[spec.Provider]
	if driver == nil {
		err := errors.New(fmt.Sprintf("Unknown provider %s", spec.Provider))
		log.Error(err, "Unknown provider")
		return nil, err
	}
	driver.Connect = spec.Connect

	log.Info("Getting master credentials")
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

	log.Info("Getting user credentials")
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
	log.Info("Getting driver")
	driver, err := p.getDriver()
	if err != nil {
		return err
	}
	log.Info("Got driver")

	log.Info(fmt.Sprintf("Current phase is %s", phase))

	switch p.Action {
	case "create":
		if (phase != "") || (phase != dbv1alpha1.Creating) || (phase != dbv1alpha1.Created) {
			return fmt.Errorf("Tried to create database, but resource %s was in unexpected status %s", p.Database, phase)
		}
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
	case "drop":
		if (phase != dbv1alpha1.DeletionRequested) || (phase != dbv1alpha1.DeletionInProgress) || (phase != dbv1alpha1.Deleted) {
			return fmt.Errorf("Tried to drop database, but resource %s was in unexpected status %s", p.Database, phase)
		}
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
	default:
		return fmt.Errorf("Unknown action %s", p.Action)
	}

	// switch {
	// case phase == "":
		
	// case phase == dbv1alpha1.Creating:
	// case phase == dbv1alpha1.DeletionRequested:
		
	// case phase == dbv1alpha1.DeletionInProgress:
	// 	err = driver.Drop(driver)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.Deleted)
	// 	if err != nil {
	// 		return err
	// 	}
	// case phase == dbv1alpha1.Deleted:
	// 	log.Info(fmt.Sprintf("Nothing to do"))
	// case phase == dbv1alpha1.BackupBeforeDeleteRequested:
	// 	err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteInProgress)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	// TODO: decide how to wrangle the writer

	// 	// err = driver.Backup(driver, writer[?])
	// 	// if err != nil {
	// 	// 	return err
	// 	// }

	// 	err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteCompleted)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	// TODO: should this then do the dropping too?

	// case phase == dbv1alpha1.BackupBeforeDeleteInProgress:
	// 	// TODO: more checking, or just initiate the backup-the-drop?
	// 	// TODO: decide how to wrangle the writer

	// 	// err = driver.Backup(driver, writer[?])
	// 	// if err != nil {
	// 	// 	return err
	// 	// }

	// 	err = PatchDatabasePhase(p.k8sclient, &p.database, dbv1alpha1.BackupBeforeDeleteCompleted)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	// TODO: should this then do the dropping too?

	// case phase == dbv1alpha1.BackupBeforeDeleteCompleted:
	// 	log.Info(fmt.Sprintf("Nothing to do"))
	// }
	return nil
}

func (p *Container) reconcileBackup() error {
	phase := p.backup.Status.Phase
	log.Info("Getting driver")
	driver, err := p.getDriver()
	if err != nil {
		return err
	}
	log.Info("Got driver")

	log.Info(fmt.Sprintf("Current phase is %s", phase))

	switch p.Action {
	case "backup":
		if (phase != "") || (phase != dbv1alpha1.Starting) {
			return fmt.Errorf("Tried to perform backup, but resource %s was in unexpected status %s", p.Backup, phase)
		}

		key := "fixme-test-key"

		awsConfig := &aws.Config{
			Region:      aws.String(p.database.Spec.BackupTo.S3.Region),
			Credentials: credentials.NewStaticCredentials(
				p.database.Spec.AwsCredentials.AccessKeyID,
				p.database.Spec.AwsCredentials.SecretAccessKey,
				""),
		}

		reader, writer := io.Pipe()
		sess, err := session.NewSession(awsConfig)
		uploader := s3manager.NewUploader(sess)
		response, err := uploader.Upload(&s3manager.UploadInput{
			Body:   reader,
			Bucket: aws.String(p.database.Spec.BackupTo.S3.Bucket),
			Key:    aws.String(key),
		})
		log.Info(fmt.Sprintf("Response: %+v", response))
		if err != nil {
			return err
		}
		err = driver.Backup(driver, writer)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unknown action %s", p.Action)
		
	}

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
	log.Info("Setup done")

	if p.Backup == "" {
		log.Info("No backup name provided, assuming action is a database action")
		return p.reconcileDatabase()
	} else {
		log.Info("Backup name provided, assuming action is a backup action")
		return p.reconcileBackup()
	}

	return nil
}
