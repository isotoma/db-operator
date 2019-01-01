# db-operator

A Kubernetes operator for managing databases. This operator does **NOT** manage the creation or deletion of database instances (i.e. actual installations of a database server). The lifecycle here is of a schema or database within an instance.

db-operator will work with databases hosted within your cluster, or elsewhere such as an Amazon RDS instance.

Conceptually db-operator decouples the lifecycle of a database **instance** from that of a **database**.  Many RDBMSes use different terminology here, but all of them support the idea of many "databases" within one "instance".

This is particularly useful when running many production or pre-production workloads within one Kubernetes cluster sharing a single database instance, which is a common deployment pattern.

## Lifecycle

This diagram shows both the phases of the database resource (ellipses) and the backup resource (in diamonds):

![State diagram](doc/state.png)

## Resources and states 

### `database`

This is a database/user/schema within a database cluster/instance.

Generally this will involve creating a user with a password, and then creating a
database owned by that user.  Implementations are provided for PostgreSQL and MySQL.

#### Database Phases

When a database resource is first created it has no `state` status. The operator delegates state changes to a `driver` which makes changes to the state as appropriate, using the `db-operator driver API`.

- **Creating**: The `driver` has begun creating the database
- **Created**: The `driver` has created the database and it is ready for use. A secret now exists, with the same name and namespace as the database, containing everything required to use it.
- **BackupRequested**: A backup of this database has been requested but has not yet begun
- **BackupInProgress**: A backup is in progress. Only one backup may be active at any one time.
- **BackupCompleted**: Backup has been completed. Will then move back to CREATED.
- **DeletionRequested**: Database deletion (without a backup) has been requested.
- **DeletionInProgress**: The database is being deleted.
- **Deleted**: The database has been deleted. At this point the database resource itself will be removed.
- **BackupBeforeDeleteRequested**: The database will be backed up and then deleted
- **BackupBeforeDeleteInProgress**: The database is being backed up before deletion
- **BackupBeforeDeleteCompleted**: The database has been backed up and will move to **DeletionRequested** shortly.

### `backup`

This is a backup of a database, stored on some remote object store such as S3.

#### Backup Phases

When a backup resource is first created it has no `state` status.

- **Starting**: The `driver` is beginning a backup.
- **BackingUp**: The `driver` is backing up. The Status will also include a destination attribute showing where the backup is being written to. It may also optionally include a progress.
- **Completed**: The backup has completed.  The resource will not be deleted automatically.

## Drivers

**Drivers** actually implement the creation, deletion and backing up of a database. How they do this is implementation specific. The `db-operator` *Driver API* contains everything required to interact with the custom resources used.

### Driver API

Your driver will be launched in a pod by a job. The following environment variables
will be set:

- **DB_OPERATOR_DATABASE** The name of the database resource
- **DB_OPERATOR_NAMESPACE** The namespace of the resources. This will also be the namespace in which the job runs.
- **DB_OPERATOR_BACKUP** The name of the backup resource, if required
- **DB_OPERATOR_OPERATION** The operation to perform

#### Drivers

The driver 

### The database resource

Example:

    driver: postgresql
    connect:
      host: db.example.com
      port: 5432
    credentials:
      username:
        value: postgres
      password:
        valueFrom:
          secretKeyRef:
            name: dbpassword
            key: password
    backupTo:
      S3:
        Region: eu-west-1
        Bucket: my-backup-bucket
        Prefix: backups/
    

