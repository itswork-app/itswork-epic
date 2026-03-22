#!/bin/bash
set -e

echo "Generating gRPC bindings for Python Brain..."

# Pastikan folder dan file init Python tersedia
mkdir -p api/proto
touch api/proto/__init__.py
touch internal/brain/__init__.py
touch cmd/brain/__init__.py

# Kompilasi CONTRACTS.proto menggunakan grpc_tools
python3 -m grpc_tools.protoc \
    -I. \
    --python_out=. \
    --grpc_python_out=. \
    api/proto/CONTRACTS.proto

echo "✅ File _pb2.py dan _pb2_grpc.py berhasil dirender di api/proto/"
