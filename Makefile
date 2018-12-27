.PHONY: doc types

doc:; make -C doc

types:; operator-sdk generate k8s
