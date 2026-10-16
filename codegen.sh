#!/usr/bin/env /bin/bash

PROTOC_PATH=$(command -v protoc)

if [[ -z "$PROTOC_PATH" ]]; then
    echo "protoc is required" >&2
    exit 1
fi

# Keep generated Go files in the repository's pb directory. source_relative
# avoids protoc's unsafe path escape check when proto files use ../pb in their
# go_package option.
"$PROTOC_PATH" -I idl --go_out=paths=source_relative:pb --go-grpc_out=paths=source_relative:pb idl/*.proto

# protobuf-ts reads imports from the same idl directory and writes the browser
# bindings beside the existing generated files.
(
    cd www
    npx "$PROTOC_PATH" --ts_out ./lib/pb -I ../idl ../idl/common.proto ../idl/api*.proto
)
