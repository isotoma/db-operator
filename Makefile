.PHONY: doc types

all: doc types
	operator-sdk build quay.io/isotoma/db-operator

doc:; make -C doc

types:; operator-sdk generate k8s
