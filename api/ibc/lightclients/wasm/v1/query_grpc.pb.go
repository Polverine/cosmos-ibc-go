// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             (unknown)
// source: ibc/lightclients/wasm/v1/query.proto

package wasmv1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	Query_Checksums_FullMethodName = "/ibc.lightclients.wasm.v1.Query/Checksums"
	Query_Code_FullMethodName      = "/ibc.lightclients.wasm.v1.Query/Code"
)

// QueryClient is the client API for Query service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type QueryClient interface {
	// Get all Wasm checksums
	Checksums(ctx context.Context, in *QueryChecksumsRequest, opts ...grpc.CallOption) (*QueryChecksumsResponse, error)
	// Get Wasm code for given checksum
	Code(ctx context.Context, in *QueryCodeRequest, opts ...grpc.CallOption) (*QueryCodeResponse, error)
}

type queryClient struct {
	cc grpc.ClientConnInterface
}

func NewQueryClient(cc grpc.ClientConnInterface) QueryClient {
	return &queryClient{cc}
}

func (c *queryClient) Checksums(ctx context.Context, in *QueryChecksumsRequest, opts ...grpc.CallOption) (*QueryChecksumsResponse, error) {
	out := new(QueryChecksumsResponse)
	err := c.cc.Invoke(ctx, Query_Checksums_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *queryClient) Code(ctx context.Context, in *QueryCodeRequest, opts ...grpc.CallOption) (*QueryCodeResponse, error) {
	out := new(QueryCodeResponse)
	err := c.cc.Invoke(ctx, Query_Code_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// QueryServer is the server API for Query service.
// All implementations must embed UnimplementedQueryServer
// for forward compatibility
type QueryServer interface {
	// Get all Wasm checksums
	Checksums(context.Context, *QueryChecksumsRequest) (*QueryChecksumsResponse, error)
	// Get Wasm code for given checksum
	Code(context.Context, *QueryCodeRequest) (*QueryCodeResponse, error)
	mustEmbedUnimplementedQueryServer()
}

// UnimplementedQueryServer must be embedded to have forward compatible implementations.
type UnimplementedQueryServer struct {
}

func (UnimplementedQueryServer) Checksums(context.Context, *QueryChecksumsRequest) (*QueryChecksumsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Checksums not implemented")
}
func (UnimplementedQueryServer) Code(context.Context, *QueryCodeRequest) (*QueryCodeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Code not implemented")
}
func (UnimplementedQueryServer) mustEmbedUnimplementedQueryServer() {}

// UnsafeQueryServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to QueryServer will
// result in compilation errors.
type UnsafeQueryServer interface {
	mustEmbedUnimplementedQueryServer()
}

func RegisterQueryServer(s grpc.ServiceRegistrar, srv QueryServer) {
	s.RegisterService(&Query_ServiceDesc, srv)
}

func _Query_Checksums_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryChecksumsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(QueryServer).Checksums(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Query_Checksums_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(QueryServer).Checksums(ctx, req.(*QueryChecksumsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Query_Code_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryCodeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(QueryServer).Code(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Query_Code_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(QueryServer).Code(ctx, req.(*QueryCodeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Query_ServiceDesc is the grpc.ServiceDesc for Query service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Query_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ibc.lightclients.wasm.v1.Query",
	HandlerType: (*QueryServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Checksums",
			Handler:    _Query_Checksums_Handler,
		},
		{
			MethodName: "Code",
			Handler:    _Query_Code_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ibc/lightclients/wasm/v1/query.proto",
}