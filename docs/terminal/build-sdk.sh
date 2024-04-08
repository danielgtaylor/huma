#$ wait 250
# Grab the OpenAPI spec
go run . openapi >openapi.yaml

# Install the SDK generator
go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest

# Generate the SDK
mkdir -p sdk
oapi-codegen -generate "types,client" -package sdk openapi.yaml >sdk/sdk.go

# Update project dependencies
go mod tidy
