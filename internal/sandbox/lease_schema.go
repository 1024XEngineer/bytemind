package sandbox

const LeaseSchemaV1JSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://bytemind.local/schemas/capability-lease-v1.json",
  "title": "CapabilityLeaseV1",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "version",
    "lease_id",
    "run_id",
    "scope",
    "issued_at",
    "expires_at",
    "kid",
    "approval_mode",
    "away_policy",
    "fs_read",
    "fs_write",
    "exec_allowlist",
    "network_allowlist",
    "signature"
  ],
  "properties": {
    "version": {
      "const": "v1"
    },
    "lease_id": {
      "type": "string",
      "minLength": 1
    },
    "run_id": {
      "type": "string",
      "minLength": 1
    },
    "scope": {
      "const": "run"
    },
    "issued_at": {
      "type": "string",
      "format": "date-time"
    },
    "expires_at": {
      "type": "string",
      "format": "date-time"
    },
    "kid": {
      "type": "string",
      "minLength": 1
    },
    "approval_mode": {
      "type": "string",
      "enum": ["interactive", "full_access", "away"]
    },
    "away_policy": {
      "type": "string",
      "enum": ["auto_deny_continue", "fail_fast"]
    },
    "fs_read": {
      "type": "array",
      "items": {
        "type": "string",
        "minLength": 1
      }
    },
    "fs_write": {
      "type": "array",
      "items": {
        "type": "string",
        "minLength": 1
      }
    },
    "exec_allowlist": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/execRule"
      }
    },
    "network_allowlist": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/networkRule"
      }
    },
    "signature": {
      "type": "string",
      "minLength": 1
    }
  },
  "$defs": {
    "execRule": {
      "type": "object",
      "additionalProperties": false,
      "required": ["command", "args_pattern"],
      "properties": {
        "command": {
          "type": "string",
          "minLength": 1
        },
        "args_pattern": {
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      }
    },
    "networkRule": {
      "type": "object",
      "additionalProperties": false,
      "required": ["host", "port", "scheme"],
      "properties": {
        "host": {
          "type": "string",
          "minLength": 1
        },
        "port": {
          "type": "integer",
          "minimum": 1,
          "maximum": 65535
        },
        "scheme": {
          "type": "string",
          "minLength": 1
        }
      }
    }
  }
}`
