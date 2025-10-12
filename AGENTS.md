# Repository Guidelines for Agents

## Table of Contents

1. [Commands](#commands)
2. [Code Style](#code-style)
3. [Concurrency](#concurrency)
4. [MCP Server Instructions](#mcp-server-instructions)
   - [Available MCP Servers](#available-mcp-servers)
     - [1. Gopls MCP Server](#1-gopls-mcp-server)
     - [2. DeepWiki MCP Server](#2-deepwiki-mcp-server)
     - [3. AWS Knowledge MCP Server](#3-aws-knowledge-mcp-server)
   - [Built-in Tools (Not MCP)](#built-in-tools-not-mcp)
   - [MCP & Tool Usage Best Practices](#mcp--tool-usage-best-practices)
5. [Testing Guidelines](#testing-guidelines)

## Commands

**Build**: `go build` or `make build-frontend-modern` for frontend  
**Lint**: `make lint` (runs goimports, go mod tidy, gofmt, go vet, golangci-lint, govulncheck)  
**Test all**: `go test -v ./...`  
**Test single**: `go test -run TestName ./package -v`  
**Test package**: `go test -v ./model`  
**Test race**: `go test -race ./...` (required before merges)  
**Dev server**: `make dev-modern` (runs modern template on port 3001)

## Code Style

**Imports**: Use `goimports -local module,github.com/songquanpeng/one-api`  
**Formatting**: Use `gofmt -s`  
**Line length**: Max 120 chars  
**Comments**: Every exported function/interface must have a comment starting with its name in complete sentences  
**Error handling**: Use `github.com/Laisky/errors/v2` - never return bare errors, always wrap with `errors.Wrap`/`errors.Wrapf`/`errors.WithStack`. Each error is processed once (returned OR logged, never both). Never use `err == nil`.  
**Logging**: Use `gmw.GetLogger(c)` for request-scoped logging (not global `logger.Logger`). Store in local var. Use `zap.Error(err)` not `err.Error()`.  
**Context**: Always pass and use context for lifecycle management  
**ORM**: Use `gorm.io/gorm`. Prefer SQL for reads via `Model/Find/First` + Joins. Reserve ORM for writes. Never use `clause`/`Preload`.  
**Testing**: Use `github.com/stretchr/testify/assert`. Create unit tests (`*_test.go`), not one-off scripts. Update tests when fixing bugs. Run `go test -race ./...` before merging.  
**Timezone**: Always use UTC in servers/databases/APIs  
**Date ranges**: Include entire final day (query until 00:00 next day)

## Concurrency

Multiple agents may modify code simultaneously. Preserve others' changes and report only irreconcilable conflicts.

## MCP Server Instructions

This repository integrates multiple MCP servers accessible in agent sessions. Each provides specialized capabilities for development workflows.

### Available MCP Servers

#### 1. Gopls MCP Server
**Purpose**: Go language intelligence and workspace operations  
**Instructions**: `.github/instructions/gopls.instructions.md`

**Core Workflows**:
- **Read Workflow**: `go_workspace` → `go_search` → `go_file_context` → `go_package_api`
- **Edit Workflow**: Read → `go_symbol_references` → Edit → `go_diagnostics` → Fix → `go test`

**Key Tools**:
- `gopls_go_workspace()`: Get workspace structure, modules, and package layout
- `gopls_go_search(query)`: Fuzzy search for Go symbols (max 100 results)
- `gopls_go_file_context(file)`: Summarize file's cross-file dependencies
- `gopls_go_package_api(packagePaths)`: Get package API summary
- `gopls_go_symbol_references(file, symbol)`: Find references to package-level symbols (supports `Foo`, `pkg.Bar`, `T.M` formats)
- `gopls_go_diagnostics(files)`: Check for parse/build errors

**Usage Guidelines**:
- Always start with `go_workspace` to understand project structure
- Use `go_search` for discovering symbols before reading files
- Run `go_diagnostics` after every edit operation
- Run `go test` after successful diagnostics to verify changes
- Use `go_symbol_references` before refactoring to understand impact

#### 2. DeepWiki MCP Server
**Purpose**: External repository documentation and API research  
**Instructions**: `.github/instructions/deepwiki.instructions.md`

**Core Tools**:
- `deepwiki_read_wiki_structure(repoName)`: Get documentation topics for a GitHub repo
- `deepwiki_read_wiki_contents(repoName)`: View full documentation about a repo
- `deepwiki_ask_question(repoName, question)`: Ask questions about a repository

**URL Formats Supported**:
- Full GitHub URLs: `https://github.com/owner/repo`
- Owner/repo format: `vercel/ai`, `facebook/react`
- Two-word format: `vercel ai`
- Library keywords: `react`, `typescript`, `nextjs`

**Usage Guidelines**:
- Use for researching external libraries/frameworks not in current codebase
- Start with `read_wiki_structure` to understand available documentation
- Use `ask_question` for specific technical queries about APIs
- Avoid repeated identical calls - documentation doesn't change frequently

**Example Queries**:
```
deepwiki_read_wiki_structure("openai/openai-python")
deepwiki_ask_question("vercel/ai", "How do I implement streaming chat completions?")
deepwiki_read_wiki_contents("microsoft/typescript")
```

#### 3. AWS Knowledge MCP Server
**Purpose**: AWS service documentation and regional availability

**Core Tools**:
- `mcp-aws-knowledge_aws___get_regional_availability(region, resource_type, filters?)`: Check AWS resource availability in regions
  - `resource_type`: `'api'` (SDK operations) or `'cfn'` (CloudFormation resources)
  - `filters`: e.g., `['Athena+UpdateNamedQuery']` or `['AWS::EC2::Instance']`
- `mcp-aws-knowledge_aws___list_regions()`: Get all AWS regions with IDs and names
- `mcp-aws-knowledge_aws___read_documentation(url, start_index?, max_length?)`: Fetch AWS docs as markdown
- `mcp-aws-knowledge_aws___recommend(url)`: Get related documentation (Highly Rated, New, Similar, Journey)
- `mcp-aws-knowledge_aws___search_documentation(search_phrase, limit?)`: Search AWS docs, blogs, solutions

**Usage Guidelines**:
- Verify resource availability before AWS deployments
- Use `search_documentation` to find AWS best practices
- Check `recommend` for newly released features
- For long docs, use pagination with `start_index`

**Example Queries**:
```
mcp-aws-knowledge_aws___get_regional_availability("us-east-1", "cfn", ["AWS::Lambda::Function"])
mcp-aws-knowledge_aws___search_documentation("S3 bucket versioning best practices", 10)
mcp-aws-knowledge_aws___read_documentation("https://docs.aws.amazon.com/lambda/latest/dg/lambda-invocation.html")
```

### Built-in Tools (Not MCP)

Agents also have access to built-in file and project tools:

**File Operations**:
- `read(filePath, offset?, limit?)`: Read file contents with line numbers (default: first 2000 lines)
  - `offset`: 0-based line number to start reading from
  - `limit`: Number of lines to read (default 2000)
- `write(filePath, content)`: Create or overwrite files
- `edit(filePath, oldString, newString)`: Precise string replacement
- `list(path)`: List directory contents
- `glob(pattern)`: Find files by pattern (e.g., `**/*.go`)
- `grep(pattern)`: Search file contents with regex

**Code Execution**:
- `bash(command)`: Execute shell commands for builds, tests, git operations

**Task Management**:
- `todowrite(todos)`: Create/update task lists for complex multi-step work
  - Each todo has: `id`, `content`, `status` (`pending`|`in_progress`|`completed`|`cancelled`), `priority` (`high`|`medium`|`low`)
- `todoread()`: View current task list
- `task(description, prompt, subagent_type)`: Launch specialized agents for complex tasks
  - `subagent_type: "general"`: General-purpose agent for research, code search, and multi-step tasks
  - ⚠️ **Note for Humans**: When delegating to sub-agents using the same AI model, there's no performance or quality benefit - the parent agent and sub-agent have identical capabilities. Delegation is most effective when using different model types (e.g., delegating simple search tasks to a faster/cheaper model, or complex reasoning to a more capable model). Consider whether the task truly requires delegation or can be handled directly by the current agent.
  - 💡 **Recommended for `general` type**: Use built-in tools (`read`, `glob`, `grep`, etc.) instead of `bash` for research and code search. This provides better performance, structured output, and follows the Unix Philosophy of composable tools.

**Usage Guidelines for Task Management**:
- Use for complex multi-step tasks (3+ steps) or non-trivial work
- Create todos immediately when receiving complex user requests
- Mark ONE task as `in_progress` at a time
- Update status in real-time - mark `completed` immediately after finishing each task
- Use `task` tool for open-ended searches requiring multiple rounds of globbing/grepping
- Launch multiple `task` agents concurrently for parallel research when possible

**When to Use Todo List**:
- Multi-step features requiring multiple file changes
- Bug fixes affecting multiple components
- Refactoring across multiple packages
- User provides numbered/comma-separated task lists
- Tasks requiring careful tracking and organization

**When NOT to Use Todo List**:
- Single straightforward tasks
- Trivial operations (< 3 steps)
- Purely conversational/informational requests

**Example Usage**:
```
# Complex feature implementation
todowrite([
  {"id": "1", "content": "Add dark mode toggle to settings UI", "status": "pending", "priority": "high"},
  {"id": "2", "content": "Implement dark mode state management", "status": "pending", "priority": "high"},
  {"id": "3", "content": "Update CSS styles for dark theme", "status": "pending", "priority": "medium"},
  {"id": "4", "content": "Run tests and build", "status": "pending", "priority": "high"}
])

# Launch research agent
task("Search for rate limiting patterns", "Find all rate limiting implementations in the codebase and summarize approaches", "general")
```

**Project Knowledge**:
- `.github/instructions/*.md`: Instruction files for Gopls, DeepWiki, Filesystem, Memory
- `mcp/docs/README.md`: Internal MCP server documentation

### MCP & Tool Usage Best Practices

1. **Tool Selection**: Choose the right tool for each task:
   - Go code intelligence → Gopls MCP
   - External API research → DeepWiki MCP
   - AWS documentation → AWS Knowledge MCP
   - Complex multi-step tasks → Task management tools (todowrite/task)
   - File operations → Built-in read/write/edit/list tools
   - Code search → Built-in grep/glob tools
   - Build/test/git → Built-in bash tool

2. **Workflow Integration**:
   - Start Go sessions with `gopls_go_workspace` for context
   - Create todo list with `todowrite` for complex tasks (3+ steps)
   - Mark tasks `in_progress` when starting, `completed` immediately when done
   - Use `read` before `edit` to verify file contents
   - Use `glob` + `grep` for efficient code discovery
   - Use `task` tool for open-ended searches requiring multiple rounds
   - Use `bash` for running tests, builds, and git operations
   - Consult instruction files (`.github/instructions/*.md`) for architectural patterns

3. **Error Handling**:
   - Gopls tools may fail gracefully - check return values
   - DeepWiki requires valid GitHub repository names
   - AWS Knowledge may return "NOT FOUND" for invalid resources
   - Always verify file operations by reading after write/edit

4. **Performance** (Unix Philosophy):
   - **Do one thing well**: `grep` searches content, `glob` matches file patterns
   - **Compose tools**: Use `glob` to find files, then `grep` to search within them
   - **Filter early**: Narrow down with `glob` patterns before expensive `read` operations
   - **Stream processing**: Tools process data efficiently without loading everything into memory
   - **Lazy loading**: Use `read(file, offset, limit)` to read only needed lines after `grep` finds locations
   - **Example workflow**: `glob("**/*.go")` → `grep("rate.*limit")` → `read(file, offset=166, limit=11)`
   - Cache DeepWiki results - docs don't change often
   - Batch related operations when possible

5. **Security**:
   - Never commit secrets found during file operations
   - Validate URLs before fetching external documentation
   - Review instruction files before modifying billing/pricing logic

## Testing Guidelines

- All bug fixes and features require updated unit tests
- Use `github.com/stretchr/testify/assert` for assertions
- Test files follow the pattern `*_test.go`
- Run specific tests: `go test -run TestName ./package -v`
- Run package tests: `go test -v ./package`
- Always run `go test -race ./...` before merging
- Use floating-point tolerance in tests when appropriate
