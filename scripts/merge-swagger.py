#!/usr/bin/env python3
"""Merge multiple Swagger 2.0 JSON files into a single spec with security definitions.

Usage:
    python3 merge-swagger.py \
        --input-dir '../../gen/openapi/iam/v1/*.swagger.json' \
        --output internal/delivery/httpdelivery/swagger.json \
        --title 'IAM Service API' \
        --description 'Identity and Access Management Service' \
        --host localhost:8081 \
        --public-operations AuthService_Login,AuthService_RefreshToken
"""

import argparse
import glob
import json
import sys


def merge_swagger(input_pattern, output_path, title, description, host,
                  public_operations):
    """Merge swagger files and add security definitions."""
    files = sorted(glob.glob(input_pattern))
    if not files:
        print(f"Error: no files match pattern '{input_pattern}'", file=sys.stderr)
        sys.exit(1)

    base = {
        "swagger": "2.0",
        "info": {
            "title": title,
            "version": "1.0.0",
            "description": description,
        },
        "host": host,
        "basePath": "/",
        "schemes": ["http", "https"],
        "consumes": ["application/json"],
        "produces": ["application/json"],
        "securityDefinitions": {
            "Bearer": {
                "type": "apiKey",
                "name": "Authorization",
                "in": "header",
                "description": 'Enter your Bearer token in the format: Bearer <token>',
            }
        },
        "security": [{"Bearer": []}],
        "paths": {},
        "definitions": {},
        "tags": [],
    }

    seen_tags = set()
    public_ops = set()
    if public_operations:
        public_ops = {op.strip() for op in public_operations.split(",")}

    for filepath in files:
        with open(filepath) as f:
            spec = json.load(f)

        # Merge tags (deduplicate by name)
        for tag in spec.get("tags", []):
            if tag["name"] not in seen_tags:
                seen_tags.add(tag["name"])
                base["tags"].append(tag)

        # Merge paths
        for path, path_item in spec.get("paths", {}).items():
            if path not in base["paths"]:
                base["paths"][path] = {}
            for method, operation in path_item.items():
                base["paths"][path][method] = operation
                # Mark public operations with empty security (no auth required)
                op_id = operation.get("operationId", "")
                if op_id in public_ops:
                    operation["security"] = []

        # Merge definitions (first-wins for duplicates)
        for name, definition in spec.get("definitions", {}).items():
            if name not in base["definitions"]:
                base["definitions"][name] = definition

    # Also handle prefixed definitions (e.g., financeV1DownloadTemplateResponse)
    # that reference the same type — remap them if needed

    with open(output_path, "w") as f:
        json.dump(base, f, indent=2)

    path_count = len(base["paths"])
    op_count = sum(
        1
        for methods in base["paths"].values()
        for m in methods
        if m in ("get", "post", "put", "delete", "patch")
    )
    def_count = len(base["definitions"])
    tag_count = len(base["tags"])
    public_count = sum(
        1
        for methods in base["paths"].values()
        for op in methods.values()
        if isinstance(op, dict) and op.get("security") == []
    )

    print(f"Merged {path_count} paths, {op_count} operations, "
          f"{def_count} definitions, {tag_count} tags")
    if public_ops:
        print(f"Public operations (no auth): {public_count}")
    print(f"Output: {output_path}")


def main():
    parser = argparse.ArgumentParser(
        description="Merge Swagger 2.0 JSON files with security definitions")
    parser.add_argument("--input-dir", required=True,
                        help="Glob pattern for input swagger files")
    parser.add_argument("--output", required=True,
                        help="Output file path")
    parser.add_argument("--title", required=True,
                        help="API title")
    parser.add_argument("--description", default="",
                        help="API description")
    parser.add_argument("--host", default="localhost:8080",
                        help="API host (default: localhost:8080)")
    parser.add_argument("--public-operations", default="",
                        help="Comma-separated operationIds that don't require auth")
    args = parser.parse_args()

    merge_swagger(
        input_pattern=args.input_dir,
        output_path=args.output,
        title=args.title,
        description=args.description,
        host=args.host,
        public_operations=args.public_operations,
    )


if __name__ == "__main__":
    main()
