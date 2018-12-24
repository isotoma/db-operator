# db-operator

A Kubernetes operator for managing databases. This operator does *NOT* manage the creation or deletion of database instances (i.e. actual installations of a database server). The lifecycle here is of a schema or database within an instance.

db-operator will work with databases hosted within your cluster, or elsewhere such as an Amazon RDS instance.

Conceptually db-operator decouples the lifecycle of a database *instance* from that of a *database*.  Many RDBMSes use different terminology here, but all of them support the idea of many "databases" within one "instance".

This is particularly useful when running many production or pre-production workloads within one Kubernetes cluster sharing a single database instance, which is a common deployment pattern.

## Database lifecycle

This diagram shows both the state of the database resource (rectangles) and the backup resource (in diamonds):

![State diagram](doc/state.png)