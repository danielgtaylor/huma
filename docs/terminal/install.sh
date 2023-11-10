#$ wait 250
# Set up a new Go project
mkdir my-api
cd my-api

# Initialize your project go.mod file
go mod init github.com/my-user/my-api

# Install Huma
go get github.com/danielgtaylor/huma/v2
