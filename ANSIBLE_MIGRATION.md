# Ansible Playbook Migration Guide

## Overview

The `update-network-diagram.yml` playbook needs to be updated to work with the new database-first architecture.

## Old Workflow (File-Based)

```yaml
- name: Update network diagram
  hosts: localhost
  tasks:
    - name: Generate infrastructure.yml
      script: scripts/generate-infrastructure-yaml.py

    - name: Update ConfigMap
      kubernetes.core.k8s:
        state: present
        definition:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: netdiagram-go-config
            namespace: default
          data:
            infrastructure.yml: "{{ lookup('file', 'infrastructure.yml') }}"

    - name: Restart deployment (to pick up file changes)
      kubernetes.core.k8s:
        state: present
        kind: Deployment
        name: netdiagram-go
        namespace: default
        definition:
          spec:
            template:
              metadata:
                annotations:
                  kubectl.kubernetes.io/restartedAt: "{{ ansible_date_time.iso8601 }}"
```

## New Workflow (API-Based)

```yaml
- name: Update network diagram
  hosts: localhost
  vars:
    netdiagram_url: "https://netdiagram.vanderlyn.house"

  tasks:
    - name: Generate infrastructure YAML
      script: scripts/generate-infrastructure-yaml.py
      register: infrastructure_yml
      changed_when: false

    - name: Import infrastructure to netdiagram API
      uri:
        url: "{{ netdiagram_url }}/api/import/ansible-inventory"
        method: POST
        body: "{{ lookup('file', 'infrastructure.yml') }}"
        headers:
          Content-Type: "application/x-yaml"
        body_format: raw
        status_code: 200
        validate_certs: yes
      register: import_result

    - name: Display import results
      debug:
        msg: |
          Import completed with strategy: {{ import_result.json.strategy }}
          Nodes created: {{ import_result.json.nodes_created }}
          Nodes updated: {{ import_result.json.nodes_updated }}
          Edges created: {{ import_result.json.edges_created }}
          Edges updated: {{ import_result.json.edges_updated }}
```

## Key Differences

### No ConfigMap Management
The ConfigMap is no longer needed. The database is the source of truth.

### No Deployment Restart
API imports trigger SSE events automatically, no restart needed.

### Import Strategy
Add `?strategy=replace` query parameter if you want to clear existing data:

```yaml
- name: Import infrastructure (replace mode)
  uri:
    url: "{{ netdiagram_url }}/api/import/ansible-inventory?strategy=replace"
    # ... rest of params
```

### Merge Strategy (Default)
The default `merge` strategy:
- Updates existing nodes/edges
- Preserves manually added nodes
- Useful for incremental updates

### Replace Strategy
Use `replace` when:
- Ansible inventory is the complete source of truth
- You want to remove manually added nodes
- Full infrastructure refresh is needed

## Alternative: Direct YAML Import

If you're using the generic infrastructure.yml format:

```yaml
- name: Import via YAML endpoint
  uri:
    url: "{{ netdiagram_url }}/api/import/yaml"
    method: POST
    body: "{{ lookup('file', 'infrastructure.yml') }}"
    headers:
      Content-Type: "application/x-yaml"
    body_format: raw
    status_code: 200
```

## Error Handling

```yaml
- name: Import infrastructure
  uri:
    url: "{{ netdiagram_url }}/api/import/ansible-inventory"
    method: POST
    body: "{{ lookup('file', 'infrastructure.yml') }}"
    headers:
      Content-Type: "application/x-yaml"
    body_format: raw
    status_code: 200
  register: import_result
  retries: 3
  delay: 5
  until: import_result is succeeded

- name: Handle import failure
  fail:
    msg: "Failed to import infrastructure: {{ import_result.json.error }}"
  when: import_result is failed
```

## Verification

Add verification steps to ensure import succeeded:

```yaml
- name: Verify import
  uri:
    url: "{{ netdiagram_url }}/api/nodes"
    method: GET
  register: nodes_result

- name: Check node count
  assert:
    that:
      - nodes_result.json | length > 0
    fail_msg: "No nodes found after import"
    success_msg: "Import verified: {{ nodes_result.json | length }} nodes present"
```

## Rollback Strategy

If you need to revert to a previous state:

```yaml
- name: Export current state before import
  uri:
    url: "{{ netdiagram_url }}/api/export/yaml"
    method: GET
  register: backup

- name: Save backup
  copy:
    content: "{{ backup.content }}"
    dest: "/tmp/netdiagram-backup-{{ ansible_date_time.epoch }}.yml"

- name: Import new configuration
  # ... import task

- name: Rollback on failure
  uri:
    url: "{{ netdiagram_url }}/api/import/yaml?strategy=replace"
    method: POST
    body: "{{ backup.content }}"
    headers:
      Content-Type: "application/x-yaml"
    body_format: raw
  when: import_result is failed
```

## Complete Example Playbook

```yaml
---
- name: Update Network Diagram
  hosts: localhost
  gather_facts: yes

  vars:
    netdiagram_url: "https://netdiagram.vanderlyn.house"
    infrastructure_file: "../ansible/infrastructure.yml"
    import_strategy: "merge"  # or "replace"

  tasks:
    - name: Check if infrastructure file exists
      stat:
        path: "{{ infrastructure_file }}"
      register: infra_file

    - name: Fail if file not found
      fail:
        msg: "Infrastructure file not found: {{ infrastructure_file }}"
      when: not infra_file.stat.exists

    - name: Create backup of current state
      uri:
        url: "{{ netdiagram_url }}/api/export/yaml"
        method: GET
        return_content: yes
      register: backup
      tags: backup

    - name: Save backup locally
      copy:
        content: "{{ backup.content }}"
        dest: "/tmp/netdiagram-backup-{{ ansible_date_time.epoch }}.yml"
      tags: backup

    - name: Import infrastructure data
      uri:
        url: "{{ netdiagram_url }}/api/import/ansible-inventory?strategy={{ import_strategy }}"
        method: POST
        body: "{{ lookup('file', infrastructure_file) }}"
        headers:
          Content-Type: "application/x-yaml"
        body_format: raw
        status_code: 200
        validate_certs: yes
      register: import_result
      retries: 3
      delay: 5

    - name: Display import results
      debug:
        msg:
          - "Import strategy: {{ import_result.json.strategy }}"
          - "Nodes created: {{ import_result.json.nodes_created }}"
          - "Nodes updated: {{ import_result.json.nodes_updated }}"
          - "Edges created: {{ import_result.json.edges_created }}"
          - "Edges updated: {{ import_result.json.edges_updated }}"

    - name: Verify import
      uri:
        url: "{{ netdiagram_url }}/api/graph"
        method: GET
        return_content: yes
      register: graph_result

    - name: Check graph validity
      assert:
        that:
          - graph_result.json.nodes | length > 0
          - graph_result.json.edges | length > 0
        fail_msg: "Graph verification failed: insufficient nodes or edges"
        success_msg: "Graph verified: {{ graph_result.json.nodes | length }} nodes, {{ graph_result.json.edges | length }} edges"
```

## Testing

Test the playbook locally first:

```bash
# Test with curl
curl -X POST https://netdiagram.vanderlyn.house/api/import/ansible-inventory \
  -H "Content-Type: application/x-yaml" \
  --data-binary @../ansible/infrastructure.yml

# Test with ansible-playbook
ansible-playbook playbooks/update-network-diagram.yml --check
ansible-playbook playbooks/update-network-diagram.yml
```

## Migration Checklist

- [ ] Update playbook to use API imports
- [ ] Remove ConfigMap management tasks
- [ ] Remove deployment restart tasks
- [ ] Add error handling and retries
- [ ] Add verification steps
- [ ] Test with --check mode
- [ ] Test with actual import
- [ ] Verify SSE events work (check /events endpoint)
- [ ] Update CI/CD pipelines if applicable
- [ ] Remove old ConfigMap from K8s cluster
- [ ] Update documentation

## Notes

- The netdiagram service must be running and accessible
- Consider using Ansible Vault for any API credentials if auth is added later
- The import is synchronous - playbook waits for completion
- SSE clients will receive real-time updates automatically
- No service downtime during import (database writes are quick)
