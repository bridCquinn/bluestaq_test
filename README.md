# Team-Isolated Distributed Note-Taking Engine

This repository contains a production-ready, framework-free REST service engineered in Go. The system is designed to provide highly performant, multi-tenant note capture and sharing mechanics tailored for several small, concurrent engineering and operations teams. 

---

## Technical Architecture & Design Decisions

### Zero-Dependency Standard Library Core
The backend is built entirely on Go’s native `net/http` standard library multiplexer, leveraging the enhanced structural routing patterns introduced in Go 1.22+. By designing without third-party frameworks (such as Gin or Fiber), the binary maintains absolute structural isolation. This minimizes runtime memory allocations, guarantees lightning-fast request routing, and entirely eliminates long-term dependency maintenance overhead or supply-chain security vulnerabilities.

### Thread-Isolated In-Memory Persistence
State management is handled via an optimized in-memory lookup map encapsulated by a synchronization primitive (`sync.RWMutex`). For the scale of several small teams, introducing an external database container adds unnecessary network latency, connection pooling friction, and deployment complexity. 

To maximize operational throughput, the engine splits locks based on data-access patterns. Read operations utilize shared reading locks (`RLock`), allowing multi-threaded team operations to query collections concurrently without thread blocking. Heavy data mutations (creation, updates, deletion) seamlessly escalate to mutual exclusion write locks (`Lock`) to guarantee state consistency and prevent data race conditions.

### Invariant Data Isolation & Boundary Enforcement
Multi-tenancy is treated as a fundamental, first-class structural constraint. Notes cannot be created or queried globally; they are bound to a strict `team_id` from the moment of ingestion. The application actively validates data requirements up front, returning explicit RESTful semantic error codes (`422 Unprocessable Entity`) for malformed parameters. This intentional data layout ensures that migrating the persistence layer to an isolated physical database (like PostgreSQL) requires zero changes to the underlying HTTP core logic.

### Resilient Production Sockets & Graceful Shutdown
To operate safely within production environments or container orchestrators like Kubernetes, the networking stack features hard socket timeout limits (`ReadTimeout`, `WriteTimeout`, `IdleTimeout`) to defend the system against hanging threads or resource exhaustion attacks. 

Furthermore, the server implements an asynchronous termination intercept sequence. When a shutdown signal is trapped (such as `SIGTERM` or `Ctrl+C`), the engine pauses incoming connections and initiates a graceful connection drain. It preserves a dedicated deadline context to allow in-flight HTTP requests to complete safely without truncating responses or dropping active user data packets mid-flight.

---

## Compiling, Running, and Testing the Engine

### Prerequisites
* Go 1.22 or higher installed locally.

---

## Compiling, Running, and Testing the Engine

### Prerequisites
* Go 1.22 or higher installed locally.
* **Make** build utility configured on your host path.

### Automated Workspace Lifecycle Commands
A `Makefile` is included to automate local orchestration. You can use these single-word shortcuts to build, execute, and verify the project instantly:

```bash
# Display all available workspace automation helper targets
make help

# Compile and spin up the REST engine with structured JSON logs active
make run

# Execute the entire table-driven multi-tenancy verification test suite
make test

# Automatically standardize codebase formatting and tidy local mod files
make clean
```


### 2. Execute Automated Unit Tests
The business constraints, payload structures, and multi-tenant isolation barriers are fully covered by a table-driven automated testing suite. Run the suite concurrently with detailed tracking flags:

```bash
go test -v ./...
```

---

## Live API Endpoint Integration Scripts

Open a separate terminal window while the server is active to interact with the raw REST engine using the following sequence of `curl` requests.

### Create a Note (Engineering Team)
Ingests a structured note body and returns a `201 Created` status accompanied by system-managed timestamps and an assigned resource identifier.
```bash
curl -X POST http://localhost:8080/notes \
  -H "Content-Type: application/json" \
  -d '{"team_id": "engineering", "title": "System Architecture", "content": "Transitioning architecture to framework-free Go components.", "author": "Alice"}'
```

### Create a Note (Marketing Team)
Seeds an alternate tenant resource boundary to explicitly evaluate data-isolation layers.
```bash
curl -X POST http://localhost:8080/notes \
  -H "Content-Type: application/json" \
  -d '{"team_id": "marketing", "title": "Q4 Campaign", "content": "Finalizing copy updates for our tech stack reveal.", "author": "Bob"}'
```

### Query Filtered Team Collection (Multi-Tenancy Check)
Queries the system collection while explicitly passing a boundary filter. The engine ensures that only data matched to that specific tenant is extracted; other teams' notes are securely filtered out.
```bash
curl "http://localhost:8080/notes?team_id=engineering"
```

### Fetch a Single Note by ID
Resolves a precise pointer reference map lookup for a singular entity.
```bash
curl http://localhost:8080/notes/1
```

### Update an Existing Note
Modifies the data payload dynamically while ensuring initial audit timestamps remain immutable.
```bash
curl -X PUT http://localhost:8080/notes/1 \
  -H "Content-Type: application/json" \
  -d '{"title": "System Architecture v2", "content": "Updated content: Refined system timeouts and verified clean execution."}'
```

### Delete a Note
Safely deallocates keys from the thread-isolated storage engine.
```bash
curl -X DELETE http://localhost:8080/notes/1
```
*(Note: Executing a `GET` request on `/notes/1` after this step will correctly return a `404 Not Found` response.)*
