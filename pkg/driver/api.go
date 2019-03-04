package driver

import (
	"context"
	"fmt"
	"time"
	"io"
	"os"
	"errors"
	"compress/gzip"
	"encoding/json"

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
	passwordGenerator "github.com/sethvargo/go-password/password"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

type Container struct {
	k8sclient client.Client
	backup    dbv1alpha1.Backup
	database  dbv1alpha1.Database
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
	Backup   func(*Driver) (*io.ReadCloser, error)
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
			log.Error(err, "Error getting database")
			return err
		}
	}
	if p.Backup != "" {
		log.Info("Getting backup")
		if err := p.getResource(p.Backup, &p.backup); err != nil {
			log.Error(err, "Error getting backup")
			return err
		}
		if err := p.getResource(p.backup.Spec.Database, &p.database); err != nil {
			log.Error(err, "Error getting database for backup")
			return err
		}
	}
	return nil
}

func (p *Container) readFromKubernetesSecret(s dbv1alpha1.SecretKeyRef) (string, error) {
	k8sSecret := &corev1.Secret{}
	err := p.getResource(s.Name, k8sSecret)
	if err != nil {
		return "", err
	}
	value := k8sSecret.Data[s.Key]

	if value == nil {
		return "", fmt.Errorf("No key %s found", s.Key)
	}

	stringValue := string(value)
	log.Info(fmt.Sprintf("Got string value, length %d", len(stringValue)))
	return stringValue, nil
}

func (p *Container) readFromAwsSecret(s dbv1alpha1.AwsSecretRef) (string, error) {
	awsConfig := p.getAWSConfig()

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return "", err
	}

	ssm := secretsmanager.New(sess)

	out, err := ssm.GetSecretValue(&secretsmanager.GetSecretValueInput{SecretId: &s.ARN})
	if err != nil {
		return "", err
	}

	stringData := out.SecretString

	log.Info("Got string data")

	bytesData := []byte(*stringData)

	var jsonData map[string]*json.RawMessage
	err = json.Unmarshal(bytesData, &jsonData)

	if err != nil {
		return "", err
	}

	var value string
	err = json.Unmarshal(*jsonData[s.Key], &value)

	return value, nil
}

func (p *Container) getCredential(cred dbv1alpha1.Credential) (string, error) {
	if cred.Value != "" {
		log.Info("Getting credentials from plain value")
		return cred.Value, nil
	}
	if cred.ValueFrom.SecretKeyRef.Name != "" {
		log.Info(fmt.Sprintf("Getting credentials from kubernetes secret: %s", cred.ValueFrom.SecretKeyRef.Name))
		return p.readFromKubernetesSecret(cred.ValueFrom.SecretKeyRef)
	}
	if cred.ValueFrom.AwsSecretKeyRef.ARN != "" {
		log.Info(fmt.Sprintf("Getting credentials from kubernetes secret: %s", cred.ValueFrom.AwsSecretKeyRef.ARN))
		return p.readFromAwsSecret(cred.ValueFrom.AwsSecretKeyRef)
	}
	log.Info("No credentials provided")
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

	if spec.UserCredentials.Password.ValueFrom.SecretKeyRef.GenerateIfNotExists == true {
		err = p.GeneratePasswordSecret(spec.UserCredentials.Password.ValueFrom.SecretKeyRef)
		if err != nil {
			return nil, err
		}
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

func (p *Container) GeneratePasswordSecret(secretKeyRef dbv1alpha1.SecretKeyRef) error {
	k8sSecret := &corev1.Secret{}
	err := p.getResource(secretKeyRef.Name, k8sSecret)
	found := !k8serrors.IsNotFound(err)

	if err != nil && !found {
		log.Error(err, "Error getting k8s secret, but wasn't a NotFound error")
		return err
	}

	if found {
		value := k8sSecret.Data[secretKeyRef.Key]

		if value != nil && string(value) != "" {
			log.Info("Secret already exists and has a value set")
			return nil
		}

		keys := make([]string, len(k8sSecret.Data))
		i := 0
		for k := range k8sSecret.Data {
			keys[i] = k
			i++
		}

		
		numKeys := len(keys)
		if numKeys > 1 {
			err = fmt.Errorf("Expected one or zero keys")
			log.Error(err, "Too many keys")
			return err
		}

		if numKeys == 1 {
			key := keys[0]
			if key != secretKeyRef.Key {
				err = fmt.Errorf("Expected single existing key %s to match %s", key, secretKeyRef.Key)
				log.Error(err, "Key mismatch")
				return err
			}
		}
	} else {
		k8sSecret.Name = secretKeyRef.Name
	}

	newpw, err := GeneratePassword(30)
	if err != nil {
		log.Error(err, "Error generating password")
		return err
	}

	k8sSecret.Data[secretKeyRef.Key] = []byte(newpw)
	err = p.k8sclient.Update(context.TODO(), k8sSecret)
	if err != nil {
		log.Error(err, "Error ")
		return err
	}
	return nil
}

func GeneratePassword(length int) (string, error) {
	return passwordGenerator.Generate(length, 0, 0, false, true)
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

func PatchBackupPhase(k8sclient client.Client, backup *dbv1alpha1.Backup, phase dbv1alpha1.BackupPhase) error {
	backup.Status.Phase = phase
	log.Info(fmt.Sprintf("Patching %s to %s", backup.Name, phase))
	err := k8sclient.Update(context.TODO(), backup)
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
		if (phase != "") && (phase != dbv1alpha1.Creating) && (phase != dbv1alpha1.Created) {
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
		if (phase != dbv1alpha1.DeletionRequested) && (phase != dbv1alpha1.DeletionInProgress) && (phase != dbv1alpha1.Deleted) {
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
	return nil
}

func (p *Container) getAWSConfig() *aws.Config {
	awsConfig := &aws.Config{
		Region:      aws.String(p.database.Spec.BackupTo.S3.Region),
	}

	if (p.database.Spec.AwsCredentials.AccessKeyID != "") || (p.database.Spec.AwsCredentials.SecretAccessKey != "") {
		log.Info("Using AWS credentials from database spec")
		awsConfig.Credentials = credentials.NewStaticCredentials(
			p.database.Spec.AwsCredentials.AccessKeyID,
			p.database.Spec.AwsCredentials.SecretAccessKey,
			"")
	} else {
		log.Info("Not using configured AWS credentials, relying on metadata")
	}
	return awsConfig
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
	log.Info(fmt.Sprintf("Action is %s", p.Action))

	switch p.Action {
	case "backup":
		log.Info("Performing action: backup")
		if (phase != "") && (phase != dbv1alpha1.Starting) {
			return fmt.Errorf("Tried to perform backup, but resource %s was in unexpected status %s", p.Backup, phase)
		}

		nowStr := time.Now().Format(time.RFC3339)
		key := p.database.Spec.BackupTo.S3.Prefix + "/" + nowStr + ".gzip"
		log.Info(fmt.Sprintf("Using s3 keyfrom database spec: %s", key))

		reader, writer := io.Pipe()
		log.Info("Got pipe")

		backupReader, err := driver.Backup(driver)
		if err != nil {
			log.Error(err, "Error getting backup reader")
			return err
		}

		awsConfig := p.getAWSConfig()

		c := make(chan error)

		go func() {
			gw := gzip.NewWriter(writer)
			// This effectively waits for the backup
			// reader command to finish, as it keeps
			// reading from the reader until it gets an
			// error or gets an EOF.
			bytes, err := io.Copy(gw, *backupReader)
			if err != nil {
				c <- err
				return
			}
			log.Info(fmt.Sprintf("%d bytes exported", bytes))
			gw.Close()
			writer.Close()
			c <- nil
		}()

		sess, err := session.NewSession(awsConfig)
		if err != nil {
			log.Error(err, "Error getting AWS session")
			return err
		}
		log.Info("Got AWS session")
		uploader := s3manager.NewUploader(sess)
		log.Info("Got uploader")
		bucketName := p.database.Spec.BackupTo.S3.Bucket
		log.Info(fmt.Sprintf("Uploading from backup reader to %s key %s", bucketName, key))
		response, err := uploader.Upload(&s3manager.UploadInput{
			Body:   reader,
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
		if err != nil {
			log.Error(err, "Error getting upload response")
			return err
		}
		log.Info(fmt.Sprintf("Response: %+v", response))

		log.Info("Waiting for Backup to complete...")

		err = <- c
		if err != nil {
			log.Error(err, "Error from zipping process")
			return err
		}

		log.Info("Marking backup as completed")
		err = PatchBackupPhase(p.k8sclient, &p.backup, dbv1alpha1.Completed)
		if err != nil {
			log.Error(err, "Error marking backup as completed")
			return err
		}
		log.Info("Backup completed")
		return nil
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
		log.Error(err, "Error running setup")
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
