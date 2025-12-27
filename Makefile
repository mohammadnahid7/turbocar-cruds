CURRENT_DIR=$(shell pwd)

include .env
export $(shell sed 's/=.*//' .env)

PDB_URL := postgres://$(PDB_USER):$(PDB_PASSWORD)@localhost:$(PDB_PORT)/$(PDB_NAME)?sslmode=disable

proto-gen:
	./scripts/gen-proto.sh ${CURRENT_DIR}

mig-up:
	migrate -path migrations -database '${PDB_URL}' -verbose up

mig-down:
	migrate -path migrations -database '${PDB_URL}' -verbose down

mig-force:
	migrate -path migrations -database '${PDB_URL}' -verbose force 1

create_migrate:
	@echo "Enter file name: "; \
	read filename; \
	migrate create -ext sql -dir migrations -seq $$filename
	
run:
	go run cmd/main.go
sqlc:
	sqlc generate -f storage/postgres/sqlc.yaml


google:
	mkdir -p protos/google/api
	mkdir -p protos/protoc-gen-openapiv2/options
	curl -o protos/google/api/annotations.proto https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/annotations.proto
	curl -o protos/google/api/http.proto https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/http.proto
	curl -o protos/protoc-gen-openapiv2/options/annotations.proto https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/annotations.proto
	curl -o protos/protoc-gen-openapiv2/options/openapiv2.proto https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/openapiv2.proto


proto:
	rm -f genproto/**/*.go
	rm -f doc/swagger/*.swagger.json
	mkdir -p genproto
	mkdir -p doc/swagger
	echo '<!DOCTYPE html><html><head><title>API Documentation</title><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" type="text/css" href="//unpkg.com/swagger-ui-dist@3/swagger-ui.css" /></head><body><div id="swagger-ui"></div><script src="//unpkg.com/swagger-ui-dist@3/swagger-ui-bundle.js"></script><script>const ui = SwaggerUIBundle({url: "swagger_docs.swagger.json",dom_id: "#swagger-ui",deepLinking: true,presets: [SwaggerUIBundle.presets.apis,SwaggerUIBundle.SwaggerUIStandalonePreset],plugins: [SwaggerUIBundle.plugins.DownloadUrl],})</script></body></html>' > doc/swagger/index.html
	protoc \
		--proto_path=protos --go_out=genproto --go_opt=paths=source_relative \
		--go-grpc_out=genproto --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=genproto --grpc-gateway_opt=paths=source_relative \
		--openapiv2_out=doc/swagger --openapiv2_opt=allow_merge=true,merge_file_name=swagger_docs,use_allof_for_refs=true,disable_service_tags=true,json_names_for_fields=false \
		--validate_out="lang=go,paths=source_relative:genproto" \
			protos/**/*.proto