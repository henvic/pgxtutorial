package apiv1

// Generate Protobuf and gRPC code:
//go:generate protoc --go_out=apipb --go_opt=paths=source_relative --go-grpc_out=./apipb --go-grpc_opt=paths=source_relative api.proto
