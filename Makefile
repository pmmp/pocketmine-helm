KUBECTL = kubectl

all: fmt config/crd bin/plugin-downloader bin/server-manager
fmt: pkg/client
	go fmt ./...

config/crd: $(shell find ./pkg/apis)
	go run sigs.k8s.io/controller-tools/cmd/controller-gen \
		paths=./pkg/apis/... crd output:crd:artifacts:config=./config/crd

bin/plugin-downloader: pkg/client $(shell find cmd/plugin-downloader)
	go build -o bin/plugin-downloader github.com/pmmp/pocketmine-helm/cmd/plugin-downloader
bin/server-manager: pkg/client $(shell find cmd/plugin-downloader)
	go build -o bin/server-manager github.com/pmmp/pocketmine-helm/cmd/server-manager

CODE_GENERATOR_VERSION = "v0.23.5"

pkg/client: $(shell find ./pkg/apis)
	bash $(GOPATH)/pkg/mod/k8s.io/code-generator@$(CODE_GENERATOR_VERSION)/generate-groups.sh \
		deepcopy,client,informer,lister \
		github.com/pmmp/pocketmine-helm/pkg/client \
		github.com/pmmp/pocketmine-helm/pkg/apis \
		pocketmine:v1alpha1
	touch pkg/client
	go fmt pkg/client/...

.PHONY: apply-crd

apply-crd: config/crd
	$(KUBECTL) apply -f config/crd/*