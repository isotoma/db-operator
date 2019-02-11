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

### Separation of concerns

`db-operator` determines whether or not a job needs to be created, based on the spec and status of the database or backup resource.

If a job is to be created then it creates the job, but does not itself make any changes to the resources.

Note that we trust that the jobs system works and will continue to retry on error, so the operator does not take on this responsibility.

The driver makes all of the changes to the resource, based on progress with reconciliation.

Note that driver authors do not need to know anything about kubernetes or making resource changes - this is abstracted away by the `Driver API`.

### Driver API

Drivers are launched in a pod by a job. The following environment variables are set:

- **DB_OPERATOR_DATABASE** The name of the database resource
- **DB_OPERATOR_NAMESPACE** The namespace of the resources. This will also be the namespace in which the job runs.
- **DB_OPERATOR_BACKUP** The name of the backup resource, if required
- **DB_OPERATOR_OPERATION** The operation to perform

The Driver API provides a mechanism for drivers to register with a container, which then calls driver methods as required to achieve reconciliation.

Driver methods MUST be idemopotent, since they may be executed more than once in a case where state is uncertain, due to failure during a previous reconciliation.

### The database resource

Example spec:

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
