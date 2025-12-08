# Implementation Checklist

## Refactoring Status: COMPLETE

All code has been written and is ready for testing/deployment.

## Pre-Deployment Verification

### 1. Build & Compile
- [ ] Install Go 1.22+ if not present
- [ ] Run `go mod tidy` to ensure dependencies are correct
- [ ] Run `make build` to verify compilation succeeds
- [ ] Test binary: `./netdiagram -addr :3000`

### 2. Database Testing
- [ ] Delete any existing `netdiagram.db` file
- [ ] Start server - verify new schema is created
- [ ] Check database has new tables: nodes, edges, node_positions

### 3. API Testing

#### Basic CRUD
```bash
# Create a node
curl -X POST http://localhost:3000/api/nodes \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-server",
    "type": "server",
    "label": "Test Server",
    "properties": {
      "ip": "192.168.0.100",
      "role": "test"
    }
  }'

# List nodes
curl http://localhost:3000/api/nodes

# Get specific node
curl http://localhost:3000/api/nodes/test-server

# Create an edge
curl -X POST http://localhost:3000/api/edges \
  -H "Content-Type: application/json" \
  -d '{
    "from_id": "test-server",
    "to_id": "router",
    "type": "ethernet",
    "properties": {
      "speed": "1GbE"
    }
  }'

# List edges
curl http://localhost:3000/api/edges

# Get complete graph
curl http://localhost:3000/api/graph

# Save positions
curl -X POST http://localhost:3000/api/positions \
  -H "Content-Type: application/json" \
  -d '[
    {
      "node_id": "test-server",
      "x": 100,
      "y": 200,
      "pinned": false
    }
  ]'

# Get positions
curl http://localhost:3000/api/positions

# Delete node
curl -X DELETE http://localhost:3000/api/nodes/test-server
```

#### Import/Export Testing
```bash
# Import from YAML
curl -X POST http://localhost:3000/api/import/yaml \
  -H "Content-Type: application/x-yaml" \
  --data-binary @examples/infrastructure.yml

# Import from Ansible inventory
curl -X POST http://localhost:3000/api/import/ansible-inventory \
  -H "Content-Type: application/x-yaml" \
  --data-binary @../ansible/infrastructure.yml

# Export as JSON
curl http://localhost:3000/api/export/json > graph.json

# Export as YAML
curl http://localhost:3000/api/export/yaml > graph.yml

# Export as Ansible inventory
curl http://localhost:3000/api/export/ansible-inventory > inventory.yml
```

#### SSE Testing
```bash
# In one terminal, subscribe to events
curl -N http://localhost:3000/events

# In another terminal, create/update/delete nodes
curl -X POST http://localhost:3000/api/nodes \
  -H "Content-Type: application/json" \
  -d '{"id":"sse-test","type":"server","label":"SSE Test"}'

# Verify event is received in first terminal
```

### 4. Frontend Testing
- [ ] Open http://localhost:3000 in browser
- [ ] Verify graph visualization loads
- [ ] Import data via API
- [ ] Refresh page - verify graph appears
- [ ] Test node dragging (positions should save)
- [ ] Refresh again - verify positions persisted

**Note**: Frontend may need updates to work with new node/edge format. See REFACTORING_SUMMARY.md for details.

## Docker Build

### Local Testing
```bash
# Build image
make docker

# Run container
docker run -d \
  --name netdiagram-test \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  netdiagram:latest

# Test
curl http://localhost:3000/api/graph

# Check logs
docker logs netdiagram-test

# Stop and remove
docker stop netdiagram-test
docker rm netdiagram-test
```

### Push to Registry
```bash
# Tag for Docker Hub
docker tag netdiagram:latest cwilson613/netdiagram-go:latest

# Login
docker login

# Push
docker push cwilson613/netdiagram-go:latest
```

## Kubernetes Deployment

### 1. Update Deployment Manifest

Location: `../k8s/manifests/netdiagram-go/deployment.yaml`

**Remove**:
- ConfigMap volume mount (if present)
- `--yaml` command-line argument

**Keep/Add**:
- PersistentVolumeClaim for database storage
- Environment variables if needed

Example deployment snippet:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: netdiagram-go
  namespace: default
spec:
  replicas: 1
  strategy:
    type: Recreate  # Important: SQLite doesn't support multiple writers
  template:
    spec:
      containers:
      - name: netdiagram-go
        image: cwilson613/netdiagram-go:latest
        args:
          - "-addr"
          - ":3000"
          - "-db"
          - "/data/netdiagram.db"
        ports:
        - containerPort: 3000
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: netdiagram-data
```

**Create PVC** (if doesn't exist):
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: netdiagram-data
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: local-path  # or your storage class
```

### 2. Deploy to K8s
```bash
# Apply PVC
kubectl apply -f ../k8s/manifests/netdiagram-go/pvc.yaml

# Apply new deployment
kubectl apply -f ../k8s/manifests/netdiagram-go/deployment.yaml

# Check rollout
kubectl rollout status deployment/netdiagram-go -n default

# Check logs
kubectl logs -f deployment/netdiagram-go -n default
```

### 3. Initial Data Import
```bash
# Port-forward to access API
kubectl port-forward -n default deployment/netdiagram-go 3000:3000 &

# Import infrastructure
curl -X POST http://localhost:3000/api/import/ansible-inventory \
  -H "Content-Type: application/x-yaml" \
  --data-binary @../ansible/infrastructure.yml

# Verify
curl http://localhost:3000/api/graph | jq '.nodes | length'

# Stop port-forward
kill %1
```

### 4. Test via Ingress
```bash
# Test via production URL
curl https://netdiagram.vanderlyn.house/api/graph

# Check node count
curl -s https://netdiagram.vanderlyn.house/api/graph | jq '.nodes | length'
```

## Ansible Playbook Update

Location: `../ansible/playbooks/update-network-diagram.yml`

### 1. Backup Current Playbook
```bash
cp ../ansible/playbooks/update-network-diagram.yml \
   ../ansible/playbooks/update-network-diagram.yml.backup
```

### 2. Update Playbook
Replace content with API-based workflow from `ANSIBLE_MIGRATION.md`.

### 3. Test Playbook
```bash
# Dry run
ansible-playbook ../ansible/playbooks/update-network-diagram.yml --check

# Real run
ansible-playbook ../ansible/playbooks/update-network-diagram.yml

# Verify
curl https://netdiagram.vanderlyn.house/api/graph
```

## Post-Deployment Validation

### 1. Functional Tests
- [ ] Can create nodes via API
- [ ] Can create edges via API
- [ ] Can update nodes
- [ ] Can delete nodes (and edges cascade)
- [ ] Can save/retrieve positions
- [ ] Import works (YAML and Ansible formats)
- [ ] Export works (JSON, YAML, Ansible)
- [ ] SSE events fire on changes
- [ ] Frontend displays graph correctly

### 2. Data Persistence
- [ ] Create some test data
- [ ] Restart pod: `kubectl rollout restart deployment/netdiagram-go -n default`
- [ ] Wait for pod to be ready
- [ ] Verify data still exists: `curl https://netdiagram.vanderlyn.house/api/graph`

### 3. Performance
- [ ] Import large inventory (all your hosts)
- [ ] Check response times: `curl -w "@curl-format.txt" https://netdiagram.vanderlyn.house/api/graph`
- [ ] Monitor database size: `kubectl exec -it deployment/netdiagram-go -- ls -lh /data/`

### 4. Integration
- [ ] Ansible playbook runs successfully
- [ ] Data appears in frontend
- [ ] Position changes persist
- [ ] Multiple clients can view simultaneously

## Cleanup Old Resources

Once verified working:

### 1. Remove Old ConfigMap
```bash
# Check if it exists
kubectl get configmap netdiagram-go-config -n default

# Delete if no longer needed
kubectl delete configmap netdiagram-go-config -n default
```

### 2. Clean Up Code
```bash
# Remove .old backup files
cd /home/styrene/workspace/vanderlyn/netdiagram-go
find . -name "*.old" -type f -delete

# Remove unused legacy code (optional - keep for reference initially)
# rm internal/loader/yaml.go
# rm internal/watcher/watcher.go
# rm internal/domain/host.go
# rm internal/domain/connection.go
# rm internal/domain/infrastructure.go
```

### 3. Update Dependencies
```bash
# Remove fsnotify (no longer needed)
go mod edit -dropreplace github.com/fsnotify/fsnotify
go mod tidy
```

## Known Issues / Limitations

### Frontend Updates Required
The frontend at `cmd/server/web/` was built for the old data model. It may need updates:

**Changes needed**:
- `node.group` → `node.type` (or map type to group)
- `node.title` → build from `node.properties`
- Handle new properties structure

**Test thoroughly before production use.**

### SQLite Limitations
- Single writer only (hence `strategy: Recreate` in deployment)
- Not suitable for high-concurrency writes
- Fine for this use case (infrequent updates, mostly reads)

### Missing Features
- Network scan endpoint (stub only)
- Authentication/authorization (if needed in future)
- Audit logging
- Batch operations beyond import

## Rollback Plan

If issues arise:

### Quick Rollback
```bash
# Redeploy old image
kubectl set image deployment/netdiagram-go \
  netdiagram-go=cwilson613/netdiagram-go:previous-tag \
  -n default

# Or restore old deployment manifest
kubectl apply -f ../k8s/manifests/netdiagram-go/deployment.yaml.backup
```

### Full Rollback
1. Restore old code from .old files
2. Rebuild and push old image
3. Restore ConfigMap-based workflow
4. Redeploy old Ansible playbook

## Success Criteria

✅ Deployment is successful when:

- [ ] `make build` succeeds without errors
- [ ] All API endpoints return expected responses
- [ ] Import/export works for both YAML and Ansible formats
- [ ] Data persists across pod restarts
- [ ] SSE events deliver real-time updates
- [ ] Frontend displays topology correctly
- [ ] Ansible playbook updates graph successfully
- [ ] No errors in application logs
- [ ] Database file size is reasonable (<10MB for typical homelab)

## Support & Troubleshooting

### Debug Mode
```bash
# Check pod status
kubectl get pods -n default | grep netdiagram

# View logs
kubectl logs -f deployment/netdiagram-go -n default

# Exec into pod
kubectl exec -it deployment/netdiagram-go -n default -- sh

# Check database
sqlite3 /data/netdiagram.db ".tables"
sqlite3 /data/netdiagram.db "SELECT COUNT(*) FROM nodes;"
```

### Common Issues

**Build fails with CGO errors**:
- Ensure build environment has gcc/musl-dev (for SQLite)
- Use Docker build instead: `make docker`

**Import fails**:
- Check YAML syntax
- Verify Content-Type header
- Check logs for parsing errors

**Data not persisting**:
- Verify PVC is mounted
- Check disk space
- Ensure SQLite has write permissions

**Frontend not loading**:
- Check browser console for errors
- Verify `/api/graph` returns valid JSON
- Check CORS headers

**SSE not working**:
- Test with `curl -N http://localhost:3000/events`
- Check firewall/proxy settings
- Verify nginx doesn't buffer SSE

## Documentation Updates Needed

After deployment:

- [ ] Update main README.md with new API usage
- [ ] Update CLAUDE.md with deployment changes
- [ ] Create API usage examples
- [ ] Update network diagram section in main vanderlyn README
- [ ] Add this implementation to deployment runbook

## Timeline Estimate

- Testing (local): 1-2 hours
- Docker build/push: 30 minutes
- K8s deployment: 30 minutes
- Ansible playbook update: 1 hour
- Frontend verification/fixes: 2-4 hours
- Documentation: 1 hour

**Total: ~6-9 hours** (depending on frontend updates needed)
