# Crux Agent Prompt

Get the AI agent prompt by running:

```bash
crux --prompt
```

This prints a ready-to-use prompt that guides an AI assistant to:
1. Run `crux --help` to understand the tool
2. Analyze the project structure
3. Create a config.yaml
4. Run crux
5. Use MCP to control services

## Quick Start

```bash
# Print the agent prompt
crux --prompt

# Copy the output and paste it to Cursor/Claude/etc.
```

## For Humans

If you're setting up crux manually:

```bash
# See full help with config examples
crux --help

# Generate example config.yaml
crux init

# Start services
crux
```
