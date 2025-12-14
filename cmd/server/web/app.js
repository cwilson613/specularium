(function() {
    'use strict';

    // State
    let network = null;
    let nodesDataSet = null;
    let edgesDataSet = null;
    let eventSource = null;
    let iconCache = {};

    // Zoom configuration
    const zoomConfig = {
        step: 0.2,          // Zoom step multiplier
        minScale: 0.1,      // Absolute minimum (safety bound)
        maxScale: 3.0,      // Absolute maximum (safety bound)
        dynamicMin: 0.1,    // Calculated based on graph size
        dynamicMax: 3.0,    // Calculated based on graph size
        baseScale: 1.0      // Reference scale for 100%
    };

    // Theme colors
    const theme = {
        greenBright: '#39ff14',
        greenMedium: '#32cd32',
        greenDim: '#228b22',
        greenDark: '#1a5c1a',
        greenDarker: '#0d2d0d',
        offBlack: '#0a0a0a',
        blue: '#74c0fc',
        orange: '#ffa94d',
        red: '#ff6b6b',
        teal: '#69db7c',
        yellow: '#ffd43b',
        gray: '#666666',
        gold: '#ffd700',
        amber: '#ffbf00',
        purple: '#9b59b6'
    };

    // Status colors for verification state
    const statusColors = {
        verified: theme.greenBright,
        unverified: theme.gray,
        verifying: theme.yellow,
        unreachable: theme.red,
        degraded: theme.orange
    };

    // Truth status colors
    const truthColors = {
        asserted: theme.gold,
        conflict: theme.amber
    };

    // Node type visual configuration with icons
    const nodeTypes = {
        server: {
            icon: '/icons/server.svg',
            color: theme.greenMedium,
            size: 30
        },
        switch: {
            icon: '/icons/switch.svg',
            color: theme.blue,
            size: 30
        },
        router: {
            icon: '/icons/router.svg',
            color: theme.orange,
            size: 35
        },
        access_point: {
            icon: '/icons/access_point.svg',
            color: theme.teal,
            size: 30
        },
        vm: {
            icon: '/icons/vm.svg',
            color: theme.greenDim,
            size: 28
        },
        vip: {
            icon: '/icons/vip.svg',
            color: theme.red,
            size: 28
        },
        container: {
            icon: '/icons/container.svg',
            color: theme.teal,
            size: 26
        },
        interface: {
            icon: '/icons/interface.svg',
            color: theme.purple,
            size: 18,
            satellite: true  // Indicates this node orbits its parent
        },
        unknown: {
            icon: '/icons/unknown.svg',
            color: theme.greenDim,
            size: 28
        }
    };

    // Create tinted SVG data URI from icon path and color
    async function getTintedIcon(iconPath, color) {
        const cacheKey = `${iconPath}:${color}`;
        if (iconCache[cacheKey]) {
            return iconCache[cacheKey];
        }

        try {
            const response = await fetch(iconPath);
            let svgText = await response.text();

            // Replace currentColor with the specified color
            svgText = svgText.replace(/currentColor/g, color);

            // Convert to data URI
            const dataUri = 'data:image/svg+xml;base64,' + btoa(svgText);
            iconCache[cacheKey] = dataUri;
            return dataUri;
        } catch (error) {
            console.error('Failed to load icon:', iconPath, error);
            return null;
        }
    }

    // Preload all icons
    async function preloadIcons() {
        const promises = [];
        for (const [type, config] of Object.entries(nodeTypes)) {
            promises.push(getTintedIcon(config.icon, config.color));
        }
        await Promise.all(promises);
    }

    // DOM elements
    const elements = {
        container: document.getElementById('network-container'),
        loading: document.getElementById('loading'),
        nodeCount: document.getElementById('node-count'),
        edgeCount: document.getElementById('edge-count'),
        clientCount: document.getElementById('client-count'),
        status: document.getElementById('status'),
        refreshBtn: document.getElementById('refresh-btn'),
        importBtn: document.getElementById('import-btn'),
        importFile: document.getElementById('import-file'),
        pasteBtn: document.getElementById('paste-btn'),
        pasteModal: document.getElementById('paste-modal'),
        pasteModalClose: document.getElementById('paste-modal-close'),
        pasteTextarea: document.getElementById('paste-textarea'),
        pasteCancel: document.getElementById('paste-cancel'),
        pasteSubmit: document.getElementById('paste-submit'),
        scanBtn: document.getElementById('scan-btn'),
        scanModal: document.getElementById('scan-modal'),
        scanModalClose: document.getElementById('scan-modal-close'),
        scanTargetSelect: document.getElementById('scan-target-select'),
        scanCidr: document.getElementById('scan-cidr'),
        scanHint: document.getElementById('scan-hint'),
        scanCancel: document.getElementById('scan-cancel'),
        scanSubmit: document.getElementById('scan-submit'),
        discoveryModeSection: document.getElementById('discovery-mode-section'),
        discoveryModeCheckbox: document.getElementById('discovery-mode-checkbox'),
        zoomIn: document.getElementById('zoom-in'),
        zoomOut: document.getElementById('zoom-out'),
        zoomFit: document.getElementById('zoom-fit'),
        zoomLevel: document.getElementById('zoom-level'),
        discoverBtn: document.getElementById('discover-btn'),
        clearBtn: document.getElementById('clear-btn'),
        actionsDropdown: document.getElementById('actions-dropdown'),
        actionsBtn: document.getElementById('actions-btn'),
        discoveryLog: document.getElementById('discovery-log'),
        discoveryLogContent: document.getElementById('discovery-log-content'),
        discoveryLogToggle: document.getElementById('discovery-log-toggle'),
        discoveryLogBadge: document.getElementById('discovery-log-badge'),
        // Discrepancy panel
        discrepancyPanel: document.getElementById('discrepancy-panel'),
        discrepancyPanelToggle: document.getElementById('discrepancy-panel-toggle'),
        discrepancyBadge: document.getElementById('discrepancy-badge'),
        discrepancyContent: document.getElementById('discrepancy-content'),
        // Node detail modal
        nodeDetailModal: document.getElementById('node-detail-modal'),
        nodeDetailTitle: document.getElementById('node-detail-title'),
        nodeDetailContent: document.getElementById('node-detail-content'),
        nodeDetailClose: document.getElementById('node-detail-close'),
        truthProperties: document.getElementById('truth-properties'),
        setTruthBtn: document.getElementById('set-truth-btn'),
        clearTruthBtn: document.getElementById('clear-truth-btn'),
        // Selection toolbar
        selectionToolbar: document.getElementById('selection-toolbar'),
        selectionCountValue: document.getElementById('selection-count-value'),
        mergeSelectedBtn: document.getElementById('merge-selected-btn'),
        clearSelectionBtn: document.getElementById('clear-selection-btn'),
        // Merge modal
        mergeModal: document.getElementById('merge-modal'),
        mergeModalClose: document.getElementById('merge-modal-close'),
        mergeSelectedCount: document.getElementById('merge-selected-count'),
        mergeParentName: document.getElementById('merge-parent-name'),
        mergeParentType: document.getElementById('merge-parent-type'),
        mergePreview: document.getElementById('merge-preview'),
        mergeCancel: document.getElementById('merge-cancel'),
        mergeSubmit: document.getElementById('merge-submit'),
        // Secrets management
        secretsBtn: document.getElementById('secrets-btn'),
        secretsModal: document.getElementById('secrets-modal'),
        secretsModalClose: document.getElementById('secrets-modal-close'),
        secretsList: document.getElementById('secrets-list'),
        secretsTypeFilter: document.getElementById('secrets-type-filter'),
        secretsSourceFilter: document.getElementById('secrets-source-filter'),
        secretsAddBtn: document.getElementById('secrets-add-btn'),
        // Secret edit modal
        secretEditModal: document.getElementById('secret-edit-modal'),
        secretEditTitle: document.getElementById('secret-edit-title'),
        secretEditClose: document.getElementById('secret-edit-close'),
        secretEditId: document.getElementById('secret-edit-id'),
        secretEditName: document.getElementById('secret-edit-name'),
        secretEditType: document.getElementById('secret-edit-type'),
        secretEditDescription: document.getElementById('secret-edit-description'),
        secretEditDataFields: document.getElementById('secret-edit-data-fields'),
        secretEditCancel: document.getElementById('secret-edit-cancel'),
        secretEditSubmit: document.getElementById('secret-edit-submit')
    };

    // Discovery log state
    let discoveryEntries = [];

    // Discrepancies state
    let discrepancies = [];

    // Scan targets state (loaded from /api/environment)
    let scanTargetsData = null;

    // Segmentum (subnet nebula) state - maps CIDR to color
    const segmentumColors = {};
    const segmentumPalette = [
        { base: 'rgba(57, 255, 20, 0.12)', glow: 'rgba(57, 255, 20, 0.25)' },    // Green (primary)
        { base: 'rgba(116, 192, 252, 0.10)', glow: 'rgba(116, 192, 252, 0.22)' }, // Blue
        { base: 'rgba(255, 169, 77, 0.10)', glow: 'rgba(255, 169, 77, 0.22)' },   // Orange
        { base: 'rgba(105, 219, 124, 0.10)', glow: 'rgba(105, 219, 124, 0.22)' }, // Teal
        { base: 'rgba(255, 107, 107, 0.10)', glow: 'rgba(255, 107, 107, 0.22)' }, // Red
        { base: 'rgba(186, 135, 252, 0.10)', glow: 'rgba(186, 135, 252, 0.22)' }, // Purple
    ];
    let segmentumColorIndex = 0;

    // Current selected node (for detail modal)
    let currentNodeId = null;
    let currentNodeData = null;

    // Multi-select state
    let selectedNodes = new Set();
    let isMultiSelectMode = false;

    // Initialize
    async function init() {
        elements.refreshBtn.addEventListener('click', loadGraph);
        elements.importBtn.addEventListener('click', () => elements.importFile.click());
        elements.importFile.addEventListener('change', handleImport);

        // Paste modal handlers
        elements.pasteBtn.addEventListener('click', openPasteModal);
        elements.pasteModalClose.addEventListener('click', closePasteModal);
        elements.pasteCancel.addEventListener('click', closePasteModal);
        elements.pasteSubmit.addEventListener('click', handlePasteImport);
        elements.pasteModal.addEventListener('click', (e) => {
            if (e.target === elements.pasteModal) closePasteModal();
        });

        // Scan modal handlers
        elements.scanBtn.addEventListener('click', openScanModal);
        elements.scanModalClose.addEventListener('click', closeScanModal);
        elements.scanCancel.addEventListener('click', closeScanModal);
        elements.scanSubmit.addEventListener('click', handleScan);
        elements.scanModal.addEventListener('click', (e) => {
            if (e.target === elements.scanModal) closeScanModal();
        });

        // Zoom control handlers
        elements.zoomIn.addEventListener('click', handleZoomIn);
        elements.zoomOut.addEventListener('click', handleZoomOut);
        elements.zoomFit.addEventListener('click', handleZoomFit);

        // Discovery and clear handlers
        elements.discoverBtn.addEventListener('click', handleDiscover);
        elements.clearBtn.addEventListener('click', handleClear);

        // Dropdown toggle
        elements.actionsBtn.addEventListener('click', toggleActionsDropdown);
        document.addEventListener('click', closeDropdownOnClickOutside);

        // Discovery log toggle
        elements.discoveryLogToggle.addEventListener('click', toggleDiscoveryLog);

        // Discrepancy panel toggle
        elements.discrepancyPanelToggle.addEventListener('click', toggleDiscrepancyPanel);

        // Node detail modal handlers
        elements.nodeDetailClose.addEventListener('click', closeNodeDetailModal);
        elements.nodeDetailModal.addEventListener('click', (e) => {
            if (e.target === elements.nodeDetailModal) closeNodeDetailModal();
        });
        elements.setTruthBtn.addEventListener('click', handleSetTruth);
        elements.clearTruthBtn.addEventListener('click', handleClearTruth);

        // Selection toolbar handlers
        elements.mergeSelectedBtn.addEventListener('click', openMergeModal);
        elements.clearSelectionBtn.addEventListener('click', clearSelection);

        // Merge modal handlers
        elements.mergeModalClose.addEventListener('click', closeMergeModal);
        elements.mergeCancel.addEventListener('click', closeMergeModal);
        elements.mergeSubmit.addEventListener('click', handleMerge);
        elements.mergeModal.addEventListener('click', (e) => {
            if (e.target === elements.mergeModal) closeMergeModal();
        });
        elements.mergeParentName.addEventListener('input', updateMergePreview);

        // Secrets management handlers
        if (elements.secretsBtn) {
            elements.secretsBtn.addEventListener('click', openSecretsModal);
        }
        if (elements.secretsModal) {
            elements.secretsModalClose.addEventListener('click', closeSecretsModal);
            elements.secretsModal.addEventListener('click', (e) => {
                if (e.target === elements.secretsModal) closeSecretsModal();
            });
            elements.secretsTypeFilter.addEventListener('change', loadSecrets);
            elements.secretsSourceFilter.addEventListener('change', loadSecrets);
            elements.secretsAddBtn.addEventListener('click', openSecretEditModal);
        }
        if (elements.secretEditModal) {
            elements.secretEditClose.addEventListener('click', closeSecretEditModal);
            elements.secretEditCancel.addEventListener('click', closeSecretEditModal);
            elements.secretEditSubmit.addEventListener('click', handleSecretSave);
            elements.secretEditModal.addEventListener('click', (e) => {
                if (e.target === elements.secretEditModal) closeSecretEditModal();
            });
            elements.secretEditType.addEventListener('change', updateSecretDataFields);
        }

        await preloadIcons();
        await registerClient();  // Register this browser as a client node
        await loadGraph();
        await loadDiscrepancies();
        connectSSE();
    }

    // Register this browser client as a node in the graph
    async function registerClient() {
        try {
            const response = await fetch('/api/client', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_agent: navigator.userAgent
                })
            });
            if (response.ok) {
                const data = await response.json();
                console.log('Client registered:', data);
            }
        } catch (error) {
            console.error('Failed to register client:', error);
        }
    }

    // Toggle actions dropdown
    function toggleActionsDropdown(e) {
        e.stopPropagation();
        elements.actionsDropdown.classList.toggle('open');
    }

    // Close dropdown when clicking outside
    function closeDropdownOnClickOutside(e) {
        if (!elements.actionsDropdown.contains(e.target)) {
            elements.actionsDropdown.classList.remove('open');
        }
    }

    // Close dropdown after action
    function closeDropdown() {
        elements.actionsDropdown.classList.remove('open');
    }

    // Discovery Log functions
    function toggleDiscoveryLog() {
        elements.discoveryLog.classList.toggle('collapsed');
        // Clear badge when expanding
        if (!elements.discoveryLog.classList.contains('collapsed')) {
            updateDiscoveryBadge(0);
        }
    }

    function expandDiscoveryLog() {
        elements.discoveryLog.classList.remove('collapsed');
        updateDiscoveryBadge(0);
    }

    function updateDiscoveryBadge(count) {
        if (count > 0) {
            elements.discoveryLogBadge.textContent = count;
            elements.discoveryLogBadge.classList.add('active');
        } else {
            elements.discoveryLogBadge.classList.remove('active');
        }
    }

    function clearDiscoveryLog() {
        discoveryEntries = [];
        elements.discoveryLogContent.innerHTML = '<div class="discovery-log-empty">No discovery activity</div>';
        updateDiscoveryBadge(0);
    }

    function addDiscoveryEntry(entry) {
        // Remove empty message if present
        const emptyMsg = elements.discoveryLogContent.querySelector('.discovery-log-empty');
        if (emptyMsg) emptyMsg.remove();

        // Create entry element
        const entryEl = document.createElement('div');
        entryEl.className = 'discovery-entry';

        const time = new Date().toLocaleTimeString();

        if (entry.node_id) {
            // Node progress entry
            const statusClass = entry.status || 'unknown';
            const statusLabel = (entry.status || 'unknown').toUpperCase();

            let details = [];
            if (entry.ip) details.push(`IP: ${entry.ip}`);
            if (entry.mac) details.push(`MAC: ${entry.mac}`);
            if (entry.ping && entry.latency !== undefined) details.push(`${entry.latency}ms`);
            if (entry.hostname) details.push(`DNS: ${entry.hostname}`);

            // Format services with banners
            let servicesHtml = '';
            if (entry.services && entry.services.length > 0) {
                const serviceStrs = entry.services.map(s => {
                    let str = `${s.port}/${s.service}`;
                    if (s.banner) str += ` "${s.banner.substring(0, 30)}${s.banner.length > 30 ? '...' : ''}"`;
                    return str;
                });
                servicesHtml = `<div class="discovery-entry-services">Services: ${serviceStrs.join(', ')}</div>`;
            } else if (entry.ports && entry.ports.length > 0) {
                servicesHtml = `<div class="discovery-entry-services">Ports: ${entry.ports.join(', ')}</div>`;
            }

            if (entry.error) details.push(`Error: ${entry.error}`);

            entryEl.innerHTML = `
                <div class="discovery-entry-header">
                    <span class="discovery-entry-node">${entry.node_id}</span>
                    <span class="discovery-entry-status ${statusClass}">${statusLabel}</span>
                    <span class="discovery-entry-time">${time}</span>
                </div>
                <div class="discovery-entry-details">${details.join(' | ') || 'No details'}</div>
                ${servicesHtml}
            `;
        } else if (entry.message) {
            // Summary entry (started/complete)
            entryEl.className = 'discovery-summary ' + (entry.total !== undefined && entry.verified !== undefined ? 'complete' : 'started');
            entryEl.innerHTML = `<span class="discovery-entry-time">${time}</span> ${entry.message}`;
        }

        // Add to top of log
        elements.discoveryLogContent.insertBefore(entryEl, elements.discoveryLogContent.firstChild);

        // Limit entries
        while (elements.discoveryLogContent.children.length > 50) {
            elements.discoveryLogContent.removeChild(elements.discoveryLogContent.lastChild);
        }

        // Auto-scroll to top
        elements.discoveryLogContent.scrollTop = 0;

        // Update badge if collapsed
        if (elements.discoveryLog.classList.contains('collapsed')) {
            const currentCount = parseInt(elements.discoveryLogBadge.textContent) || 0;
            updateDiscoveryBadge(currentCount + 1);
        }
    }

    // Open paste modal
    function openPasteModal() {
        closeDropdown();
        elements.pasteModal.classList.add('active');
        elements.pasteTextarea.focus();
    }

    // Close paste modal
    function closePasteModal() {
        elements.pasteModal.classList.remove('active');
        elements.pasteTextarea.value = '';
    }

    // Open scan modal - loads suggested targets from environment
    async function openScanModal() {
        closeDropdown();
        elements.scanModal.classList.add('active');

        // Reset discovery mode checkbox
        elements.discoveryModeCheckbox.checked = false;

        // Load environment to get suggested scan targets
        try {
            const response = await fetch('/api/environment');
            if (response.ok) {
                scanTargetsData = await response.json();
                populateScanTargets(false);
            } else {
                // Fallback to manual entry
                showManualScanInput();
            }
        } catch (error) {
            console.error('Failed to load environment:', error);
            showManualScanInput();
        }

        // Setup discovery mode checkbox handler
        elements.discoveryModeCheckbox.onchange = () => {
            populateScanTargets(elements.discoveryModeCheckbox.checked);
        };
    }

    // Populate scan target dropdown with detected subnets
    // includeDiscovery: boolean - whether to include RFC1918 discovery targets
    function populateScanTargets(includeDiscovery) {
        if (!scanTargetsData) {
            showManualScanInput();
            return;
        }

        const select = elements.scanTargetSelect;
        select.innerHTML = '';

        const scanTargets = scanTargetsData.scan_targets || {};
        const primaryTargets = scanTargets.primary || [];
        const discoveryTargets = scanTargets.discovery || [];

        // Check if we have any primary targets
        const hasPrimary = primaryTargets.length > 0;
        const hasDiscovery = discoveryTargets.length > 0;

        // Show discovery mode section if there are discovery targets available
        if (hasDiscovery) {
            elements.discoveryModeSection.style.display = 'block';
        } else {
            elements.discoveryModeSection.style.display = 'none';
        }

        // Add primary (configured/detected) targets
        if (hasPrimary) {
            const primaryGroup = document.createElement('optgroup');
            primaryGroup.label = 'CONFIGURED / DETECTED';
            primaryTargets.forEach((target) => {
                const opt = document.createElement('option');
                opt.value = target;
                opt.textContent = target;
                primaryGroup.appendChild(opt);
            });
            select.appendChild(primaryGroup);
        }

        // Add discovery targets if enabled
        if (includeDiscovery && hasDiscovery) {
            const discoveryGroup = document.createElement('optgroup');
            discoveryGroup.label = 'DISCOVERY (RFC1918)';
            discoveryTargets.forEach((target) => {
                const opt = document.createElement('option');
                opt.value = target;
                opt.textContent = target;
                opt.className = 'discovery-target';
                discoveryGroup.appendChild(opt);
            });
            select.appendChild(discoveryGroup);
        }

        // Add custom option
        const customOpt = document.createElement('option');
        customOpt.value = '__custom__';
        customOpt.textContent = 'Custom CIDR...';
        select.appendChild(customOpt);

        // If no targets at all, show manual input
        if (!hasPrimary && (!includeDiscovery || !hasDiscovery)) {
            showManualScanInput();
            return;
        }

        // Show dropdown, hide manual input
        select.style.display = 'block';
        elements.scanCidr.style.display = 'none';

        if (hasPrimary) {
            elements.scanHint.textContent = 'Select a configured subnet or enable Discovery Mode for broader scanning.';
        } else {
            elements.scanHint.textContent = 'No subnets configured. Enable Discovery Mode or enter a custom CIDR.';
        }

        // Handle custom selection
        select.onchange = () => {
            if (select.value === '__custom__') {
                showManualScanInput();
            }
        };
    }

    // Show manual CIDR input
    function showManualScanInput() {
        elements.scanTargetSelect.style.display = 'none';
        elements.scanCidr.style.display = 'block';
        elements.scanCidr.value = '192.168.0.0/24';
        elements.scanCidr.focus();
        elements.scanCidr.select();
        elements.scanHint.textContent = 'Enter a CIDR notation subnet to scan (e.g., 192.168.0.0/24)';
        // Still show discovery section if we have discovery targets
        if (scanTargetsData?.scan_targets?.discovery?.length > 0) {
            elements.discoveryModeSection.style.display = 'block';
        }
    }

    // Close scan modal
    function closeScanModal() {
        elements.scanModal.classList.remove('active');
    }

    // Handle network scan
    async function handleScan() {
        // Get CIDR from either dropdown or manual input
        let cidr;
        if (elements.scanTargetSelect.style.display !== 'none' &&
            elements.scanTargetSelect.value &&
            elements.scanTargetSelect.value !== '__custom__') {
            cidr = elements.scanTargetSelect.value;
        } else {
            cidr = elements.scanCidr.value.trim();
        }

        if (!cidr) {
            updateStatus('ERROR: NO SUBNET SELECTED');
            return;
        }

        try {
            elements.scanSubmit.disabled = true;
            updateStatus('STARTING SCAN');

            // Clear and expand the discovery log
            clearDiscoveryLog();
            expandDiscoveryLog();

            const response = await fetch('/api/import/scan', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ cidr: cidr })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.details || error.error || `HTTP ${response.status}`);
            }

            updateStatus(`SCANNING ${cidr}`);
            closeScanModal();

        } catch (error) {
            console.error('Scan failed:', error);
            updateStatus('SCAN ERROR: ' + error.message);
        } finally {
            elements.scanSubmit.disabled = false;
        }
    }

    // Handle paste import
    async function handlePasteImport() {
        const content = elements.pasteTextarea.value.trim();
        if (!content) {
            updateStatus('ERROR: NO YAML PROVIDED');
            return;
        }

        try {
            elements.pasteSubmit.disabled = true;
            updateStatus('IMPORTING');

            const response = await fetch('/api/import/ansible-inventory', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: content
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            const result = await response.json();
            updateStatus(`IMPORTED ${result.nodes_created || 0} NODES`);

            closePasteModal();
            await loadGraph();

        } catch (error) {
            console.error('Import failed:', error);
            updateStatus('IMPORT ERROR: ' + error.message);
        } finally {
            elements.pasteSubmit.disabled = false;
        }
    }

    // Handle discovery trigger
    async function handleDiscover() {
        closeDropdown();
        try {
            elements.discoverBtn.disabled = true;
            updateStatus('TRIGGERING DISCOVERY');

            // Clear and expand the discovery log
            clearDiscoveryLog();
            expandDiscoveryLog();

            const response = await fetch('/api/discover', {
                method: 'POST'
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            updateStatus('DISCOVERY INITIATED');

        } catch (error) {
            console.error('Discovery failed:', error);
            updateStatus('DISCOVERY ERROR: ' + error.message);
        } finally {
            elements.discoverBtn.disabled = false;
        }
    }

    // Handle clear graph
    async function handleClear() {
        closeDropdown();
        // Confirm before clearing
        if (!confirm('Are you sure you want to clear all nodes and edges? This cannot be undone.')) {
            return;
        }

        try {
            elements.clearBtn.disabled = true;
            updateStatus('CLEARING GRAPH');

            const response = await fetch('/api/graph', {
                method: 'DELETE'
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            updateStatus('GRAPH CLEARED');
            await loadGraph();

        } catch (error) {
            console.error('Clear failed:', error);
            updateStatus('CLEAR ERROR: ' + error.message);
        } finally {
            elements.clearBtn.disabled = false;
        }
    }

    // Handle file import
    async function handleImport(event) {
        closeDropdown();
        const file = event.target.files[0];
        if (!file) return;

        try {
            elements.importBtn.disabled = true;
            updateStatus('IMPORTING');

            const content = await file.text();
            const response = await fetch('/api/import/ansible-inventory', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: content
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            const result = await response.json();
            updateStatus(`IMPORTED ${result.nodes_created || 0} NODES`);

            // Reload the graph
            await loadGraph();

        } catch (error) {
            console.error('Import failed:', error);
            updateStatus('IMPORT ERROR: ' + error.message);
        } finally {
            elements.importBtn.disabled = false;
            elements.importFile.value = ''; // Reset file input
        }
    }

    // Load graph data from API
    async function loadGraph() {
        closeDropdown();
        try {
            updateStatus('LOADING');
            elements.loading.classList.remove('hidden');

            const response = await fetch('/api/graph');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const graph = await response.json();
            renderNetwork(graph);

            elements.loading.classList.add('hidden');
            updateStatus('OPERATIONAL');

        } catch (error) {
            console.error('Failed to load graph:', error);
            elements.loading.classList.add('hidden');
            updateStatus('ERROR: ' + error.message);
        }
    }

    // Render network graph with vis-network
    async function renderNetwork(graph) {
        const nodeCount = graph.nodes ? graph.nodes.length : 0;
        const edgeCount = graph.edges ? graph.edges.length : 0;
        elements.nodeCount.textContent = String(nodeCount).padStart(2, '0');
        elements.edgeCount.textContent = String(edgeCount).padStart(2, '0');

        // Build parent-child lookup for satellite positioning
        const childrenByParent = {};
        const nodeById = {};
        (graph.nodes || []).forEach(n => {
            nodeById[n.id] = n;
            if (n.parent_id) {
                if (!childrenByParent[n.parent_id]) {
                    childrenByParent[n.parent_id] = [];
                }
                childrenByParent[n.parent_id].push(n.id);
            }
        });

        // Transform nodes to vis-network format with icons
        const nodes = await Promise.all((graph.nodes || []).map(async n => {
            const typeConfig = nodeTypes[n.type] || nodeTypes.unknown;
            const position = graph.positions && graph.positions[n.id];
            const isSatellite = typeConfig.satellite && n.parent_id;

            // Determine border color based on verification status and truth status
            const status = n.status || 'unverified';
            const truthStatus = n.truth_status || '';

            // Truth status takes precedence for border color
            let borderColor;
            if (truthStatus === 'conflict') {
                borderColor = truthColors.conflict;  // Amber for conflicts
            } else if (truthStatus === 'asserted') {
                borderColor = truthColors.asserted;  // Gold for asserted truth
            } else {
                borderColor = statusColors[status] || statusColors.unverified;
            }

            // Tint icon based on status - use dimmer color for unverified/unreachable
            const iconColor = (status === 'verified' || status === 'degraded')
                ? typeConfig.color
                : theme.gray;
            const iconDataUri = await getTintedIcon(typeConfig.icon, iconColor);

            // Add truth indicator to label
            let label = (n.label || n.id).toUpperCase();
            if (truthStatus === 'asserted') {
                label = '\u2713 ' + label;  // Checkmark for truth
            } else if (truthStatus === 'conflict') {
                label = '\u26A0 ' + label;  // Warning for conflict
            }

            // Satellite nodes get smaller labels
            const fontSize = isSatellite ? 10 : 14;

            return {
                id: n.id,
                label: label,
                title: buildTooltip(n),
                shape: 'circularImage',
                image: iconDataUri,
                size: typeConfig.size,
                color: {
                    border: borderColor,
                    background: theme.offBlack,
                    highlight: {
                        border: theme.greenBright,
                        background: theme.greenDarker
                    },
                    hover: {
                        border: theme.greenBright,
                        background: theme.greenDarker
                    }
                },
                borderWidth: truthStatus ? 3 : 2,  // Thicker border for truth nodes
                borderWidthSelected: 4,
                font: {
                    color: borderColor,
                    face: 'VT323, monospace',
                    size: fontSize,
                    vadjust: isSatellite ? 4 : 8
                },
                x: position ? position.x : undefined,
                y: position ? position.y : undefined,
                physics: position ? !position.pinned : true,
                // Store satellite metadata for drag handling
                _isSatellite: isSatellite,
                _parentId: n.parent_id || null,
                // Store segmentum (subnet) for nebula rendering
                _segmentum: n.properties?.segmentum || null,
                _ip: n.properties?.ip || null,
                // Keep full properties for getNodeSegmentum fallback
                properties: n.properties
            };
        }));

        // Transform edges to vis-network format
        const edges = (graph.edges || []).map(e => {
            return {
                id: e.id || `${e.from_id}-${e.to_id}`,
                from: e.from_id,
                to: e.to_id,
                color: {
                    color: theme.greenMedium,
                    highlight: theme.greenBright,
                    hover: theme.greenBright
                },
                width: 2,
                smooth: {
                    enabled: true,
                    type: 'continuous',
                    roundness: 0.5
                }
            };
        });

        // Create synthetic edges between satellite nodes and their parents
        const satelliteEdges = [];
        (graph.nodes || []).forEach(n => {
            if (n.parent_id && nodeById[n.parent_id]) {
                satelliteEdges.push({
                    id: `_satellite_${n.id}`,
                    from: n.parent_id,
                    to: n.id,
                    color: {
                        color: theme.purple,
                        highlight: theme.purple,
                        hover: theme.purple
                    },
                    width: 1,
                    dashes: [2, 4],  // Dotted line to distinguish from real edges
                    length: 60,  // Short fixed length to keep satellites close
                    smooth: false,
                    _isSatelliteEdge: true
                });
            }
        });

        const allEdges = [...edges, ...satelliteEdges];

        // Create or update DataSets
        if (nodesDataSet) {
            nodesDataSet.clear();
            nodesDataSet.add(nodes);
        } else {
            nodesDataSet = new vis.DataSet(nodes);
        }

        if (edgesDataSet) {
            edgesDataSet.clear();
            edgesDataSet.add(allEdges);
        } else {
            edgesDataSet = new vis.DataSet(allEdges);
        }

        // Create network if not exists
        if (!network) {
            const options = {
                nodes: {
                    font: {
                        color: theme.greenBright,
                        face: 'VT323, monospace',
                        size: 16
                    },
                    margin: { top: 10, right: 14, bottom: 10, left: 14 }
                },
                edges: {
                    color: {
                        color: theme.greenMedium,
                        highlight: theme.greenBright,
                        hover: theme.greenBright
                    },
                    width: 2,
                    smooth: {
                        enabled: true,
                        type: 'continuous',
                        roundness: 0.5
                    }
                },
                physics: {
                    enabled: true,
                    solver: 'barnesHut',
                    barnesHut: {
                        gravitationalConstant: -3000,
                        centralGravity: 0.1,
                        springLength: 150,
                        springConstant: 0.02,
                        damping: 0.09,
                        avoidOverlap: 0.5
                    },
                    stabilization: {
                        enabled: true,
                        iterations: 200,
                        updateInterval: 25,
                        fit: true
                    }
                },
                interaction: {
                    dragNodes: true,
                    dragView: true,
                    zoomView: true,
                    zoomSpeed: 0.5,
                    hover: true,
                    tooltipDelay: 200,
                    hideEdgesOnDrag: false,
                    hideEdgesOnZoom: false
                },
                layout: {
                    improvedLayout: true,
                    hierarchical: false
                }
            };

            network = new vis.Network(
                elements.container,
                { nodes: nodesDataSet, edges: edgesDataSet },
                options
            );

            // Constrain satellite nodes to orbit radius during drag
            const SATELLITE_ORBIT_RADIUS = 60;  // Distance from parent

            network.on('dragging', (params) => {
                if (params.nodes.length === 0) return;

                params.nodes.forEach(nodeId => {
                    const nodeData = nodesDataSet.get(nodeId);
                    if (!nodeData || !nodeData._isSatellite || !nodeData._parentId) return;

                    // Get parent position
                    const parentPos = network.getPositions([nodeData._parentId])[nodeData._parentId];
                    if (!parentPos) return;

                    // Get current drag position
                    const currentPos = network.getPositions([nodeId])[nodeId];
                    if (!currentPos) return;

                    // Calculate vector from parent to current position
                    const dx = currentPos.x - parentPos.x;
                    const dy = currentPos.y - parentPos.y;
                    const distance = Math.sqrt(dx * dx + dy * dy);

                    // If too close or too far, constrain to orbit radius
                    if (distance < SATELLITE_ORBIT_RADIUS * 0.5 || distance > SATELLITE_ORBIT_RADIUS * 1.5) {
                        const angle = Math.atan2(dy, dx);
                        const newX = parentPos.x + Math.cos(angle) * SATELLITE_ORBIT_RADIUS;
                        const newY = parentPos.y + Math.sin(angle) * SATELLITE_ORBIT_RADIUS;
                        network.moveNode(nodeId, newX, newY);
                    }
                });
            });

            // Save position when drag ends (with orbit constraint for satellites)
            network.on('dragEnd', (params) => {
                if (params.nodes.length > 0) {
                    params.nodes.forEach(nodeId => {
                        const nodeData = nodesDataSet.get(nodeId);
                        const positions = network.getPositions([nodeId]);

                        if (positions[nodeId]) {
                            // For satellite nodes, enforce final orbit constraint
                            if (nodeData && nodeData._isSatellite && nodeData._parentId) {
                                const parentPos = network.getPositions([nodeData._parentId])[nodeData._parentId];
                                if (parentPos) {
                                    const dx = positions[nodeId].x - parentPos.x;
                                    const dy = positions[nodeId].y - parentPos.y;
                                    const angle = Math.atan2(dy, dx);
                                    const finalX = parentPos.x + Math.cos(angle) * SATELLITE_ORBIT_RADIUS;
                                    const finalY = parentPos.y + Math.sin(angle) * SATELLITE_ORBIT_RADIUS;
                                    network.moveNode(nodeId, finalX, finalY);
                                    savePosition(nodeId, { x: finalX, y: finalY });
                                    return;
                                }
                            }
                            savePosition(nodeId, positions[nodeId]);
                        }
                    });
                }
            });

            // Open node detail modal on double-click
            network.on('doubleClick', (params) => {
                if (params.nodes.length > 0) {
                    const nodeId = params.nodes[0];
                    openNodeDetailModal(nodeId);
                }
            });

            // Multi-select with Ctrl/Cmd+Click or Shift+Click
            network.on('click', (params) => {
                if (params.nodes.length > 0) {
                    const nodeId = params.nodes[0];
                    const isModifierHeld = params.event.srcEvent.ctrlKey ||
                                          params.event.srcEvent.metaKey ||
                                          params.event.srcEvent.shiftKey;

                    if (isModifierHeld) {
                        // Toggle node in selection
                        if (selectedNodes.has(nodeId)) {
                            selectedNodes.delete(nodeId);
                        } else {
                            selectedNodes.add(nodeId);
                        }
                        updateSelectionUI();
                    } else if (selectedNodes.size > 0) {
                        // Regular click clears selection unless clicking a selected node
                        if (!selectedNodes.has(nodeId)) {
                            clearSelection();
                        }
                    }
                } else {
                    // Clicked empty space - clear selection
                    if (selectedNodes.size > 0 && !params.event.srcEvent.ctrlKey &&
                        !params.event.srcEvent.metaKey && !params.event.srcEvent.shiftKey) {
                        clearSelection();
                    }
                }
            });

            // Fit after stabilization and calculate zoom bounds
            network.on('stabilizationIterationsDone', () => {
                network.fit({ animation: { duration: 500, easingFunction: 'easeInOutQuad' } });
                // Calculate bounds after positions are stable
                setTimeout(() => {
                    calculateZoomBounds();
                    updateZoomDisplay();
                }, 600);
            });

            // Update zoom display when user zooms with mouse wheel or touch
            // Also enforce zoom limits
            network.on('zoom', () => {
                const currentScale = network.getScale();

                // Clamp if outside bounds (with small tolerance to avoid jitter)
                if (currentScale < zoomConfig.dynamicMin * 0.99) {
                    network.moveTo({ scale: zoomConfig.dynamicMin });
                } else if (currentScale > zoomConfig.dynamicMax * 1.01) {
                    network.moveTo({ scale: zoomConfig.dynamicMax });
                }

                updateZoomDisplay();
            });

            // Segmentum (subnet nebula) rendering - draw galactic regions behind nodes
            network.on('beforeDrawing', (ctx) => {
                drawSegmenta(ctx);
            });

            // Recalculate bounds when window is resized
            window.addEventListener('resize', () => {
                calculateZoomBounds();
                updateZoomDisplay();
            });
        } else {
            // Network already exists, recalculate bounds
            setTimeout(() => {
                calculateZoomBounds();
                updateZoomDisplay();
            }, 100);
        }
    }

    // Build tooltip text for a node (plain text, no HTML - vis-network doesn't render HTML in tooltips)
    function buildTooltip(node) {
        let text = `${(node.label || node.id).toUpperCase()}\n`;
        text += `${'─'.repeat(20)}\n`;
        text += `Type: ${node.type || 'unknown'}\n`;

        // Show parent relationship for interface nodes
        if (node.parent_id) {
            text += `Parent: ${node.parent_id}\n`;
        }

        // Status indicator
        const status = node.status || 'unverified';
        const statusIcon = {
            verified: '[OK]',
            unverified: '[?]',
            verifying: '[...]',
            unreachable: '[X]',
            degraded: '[!]'
        }[status] || '[?]';
        text += `Status: ${statusIcon} ${status.toUpperCase()}\n`;

        if (node.properties) {
            if (node.properties.ip) text += `IP: ${node.properties.ip}\n`;
            if (node.properties.description) text += `${node.properties.description}\n`;
        }

        // Discovered info
        if (node.discovered) {
            if (node.discovered.mac_address) {
                text += `MAC: ${node.discovered.mac_address}\n`;
            }
            if (node.discovered.ping_latency_ms !== undefined) {
                text += `Latency: ${node.discovered.ping_latency_ms}ms\n`;
            }
            if (node.discovered.icmp_latency_ms !== undefined) {
                text += `ICMP: ${node.discovered.icmp_latency_ms}ms\n`;
            }
            // Show services with banners if available
            if (node.discovered.services && node.discovered.services.length > 0) {
                text += `Services:\n`;
                node.discovered.services.forEach(s => {
                    text += `  ${s.port}/${s.service}`;
                    if (s.banner) text += ` - ${s.banner.substring(0, 40)}`;
                    text += `\n`;
                });
            } else if (node.discovered.open_ports && node.discovered.open_ports.length > 0) {
                text += `Ports: ${node.discovered.open_ports.join(', ')}\n`;
            }
            if (node.discovered.reverse_dns) {
                text += `DNS: ${node.discovered.reverse_dns}\n`;
            }
        }

        // Last verified timestamp
        if (node.last_verified) {
            const lastVerified = new Date(node.last_verified);
            text += `Verified: ${lastVerified.toLocaleTimeString()}\n`;
        }

        // Truth status
        if (node.truth_status) {
            const truthIcon = node.truth_status === 'conflict' ? '[!]' : '[T]';
            text += `\nTruth: ${truthIcon} ${node.truth_status.toUpperCase()}\n`;
            if (node.truth && node.truth.properties) {
                text += `Locked Properties:\n`;
                for (const [key, value] of Object.entries(node.truth.properties)) {
                    text += `  ${key}: ${value}\n`;
                }
            }
            if (node.has_discrepancy) {
                text += `\n[!] HERESY DETECTED - Purge required\n`;
            }
        }

        return text;
    }

    // Save node position to API
    async function savePosition(nodeId, position) {
        try {
            await fetch(`/api/positions/${nodeId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    node_id: nodeId,
                    x: position.x,
                    y: position.y,
                    pinned: false
                })
            });
        } catch (error) {
            console.error('Failed to save position:', error);
        }
    }

    // Calculate dynamic zoom bounds based on graph dimensions
    function calculateZoomBounds() {
        if (!network || !nodesDataSet || nodesDataSet.length === 0) {
            zoomConfig.dynamicMin = zoomConfig.minScale;
            zoomConfig.dynamicMax = zoomConfig.maxScale;
            return;
        }

        // Get the bounding box of all nodes
        const positions = network.getPositions();
        const nodeIds = Object.keys(positions);

        if (nodeIds.length === 0) {
            zoomConfig.dynamicMin = zoomConfig.minScale;
            zoomConfig.dynamicMax = zoomConfig.maxScale;
            return;
        }

        let minX = Infinity, maxX = -Infinity;
        let minY = Infinity, maxY = -Infinity;

        nodeIds.forEach(id => {
            const pos = positions[id];
            if (pos.x < minX) minX = pos.x;
            if (pos.x > maxX) maxX = pos.x;
            if (pos.y < minY) minY = pos.y;
            if (pos.y > maxY) maxY = pos.y;
        });

        // Graph dimensions with padding for node sizes
        const nodePadding = 100; // Account for node radius and labels
        const graphWidth = (maxX - minX) + nodePadding * 2;
        const graphHeight = (maxY - minY) + nodePadding * 2;

        // Viewport dimensions
        const viewportWidth = elements.container.clientWidth;
        const viewportHeight = elements.container.clientHeight;

        // Calculate min zoom: should show entire graph with some padding
        // The scale at which the graph exactly fits the viewport
        const fitScaleX = viewportWidth / graphWidth;
        const fitScaleY = viewportHeight / graphHeight;
        const fitScale = Math.min(fitScaleX, fitScaleY);

        // Allow zooming out to 50% of fit scale (to see more context)
        // but never below absolute minimum
        zoomConfig.dynamicMin = Math.max(fitScale * 0.5, zoomConfig.minScale);

        // Calculate max zoom based on graph density
        // For sparse graphs (few nodes), allow more zoom in
        // For dense graphs, limit zoom to prevent single-node views
        const nodeCount = nodeIds.length;
        const graphArea = graphWidth * graphHeight;
        const density = nodeCount / (graphArea / 10000); // nodes per 100x100 area

        // Base max zoom on density:
        // - Low density (< 0.5): allow up to 3x
        // - Medium density (0.5-2): allow 2-2.5x
        // - High density (> 2): allow 1.5-2x
        let densityMaxZoom;
        if (density < 0.5) {
            densityMaxZoom = 3.0;
        } else if (density < 2) {
            densityMaxZoom = 2.5 - (density - 0.5) * 0.33;
        } else {
            densityMaxZoom = Math.max(1.5, 2.0 - (density - 2) * 0.1);
        }

        // Also limit based on minimum useful view (at least 2-3 nodes visible)
        // Assuming avg node spacing, max zoom where multiple nodes are visible
        const avgSpacing = Math.sqrt(graphArea / nodeCount);
        const minVisibleNodes = 3;
        const maxZoomForVisibility = viewportWidth / (avgSpacing * minVisibleNodes);

        zoomConfig.dynamicMax = Math.min(
            Math.max(densityMaxZoom, maxZoomForVisibility),
            zoomConfig.maxScale
        );

        // Store the fit scale for the fit button
        zoomConfig.fitScale = fitScale;

        // Determine base scale (what "100%" means)
        // Use fit scale as 100% so zoom percentage is intuitive
        zoomConfig.baseScale = fitScale;
    }

    // Update zoom level display and button states
    function updateZoomDisplay() {
        if (!network) return;

        const currentScale = network.getScale();
        let percentage = Math.round((currentScale / zoomConfig.baseScale) * 100);

        // Snap to 100% if within 2% tolerance (handles floating point precision)
        if (percentage >= 98 && percentage <= 102) {
            const exactRatio = currentScale / zoomConfig.baseScale;
            if (Math.abs(exactRatio - 1.0) < 0.02) {
                percentage = 100;
            }
        }

        elements.zoomLevel.textContent = percentage + '%';

        // Update button disabled states based on bounds
        elements.zoomIn.disabled = currentScale >= zoomConfig.dynamicMax;
        elements.zoomOut.disabled = currentScale <= zoomConfig.dynamicMin;
    }

    // Zoom in handler
    function handleZoomIn() {
        if (!network) return;

        const currentScale = network.getScale();
        let newScale = currentScale * (1 + zoomConfig.step);

        // Clamp to dynamic max
        newScale = Math.min(newScale, zoomConfig.dynamicMax);

        network.moveTo({
            scale: newScale,
            animation: { duration: 200, easingFunction: 'easeInOutQuad' }
        });

        // Update display after animation completes
        setTimeout(updateZoomDisplay, 210);
    }

    // Zoom out handler
    function handleZoomOut() {
        if (!network) return;

        const currentScale = network.getScale();
        let newScale = currentScale / (1 + zoomConfig.step);

        // Clamp to dynamic min
        newScale = Math.max(newScale, zoomConfig.dynamicMin);

        network.moveTo({
            scale: newScale,
            animation: { duration: 200, easingFunction: 'easeInOutQuad' }
        });

        // Update display after animation completes
        setTimeout(updateZoomDisplay, 210);
    }

    // Fit to view handler
    function handleZoomFit() {
        if (!network) return;

        network.fit({
            animation: { duration: 300, easingFunction: 'easeInOutQuad' }
        });

        // Update display after animation completes
        setTimeout(updateZoomDisplay, 310);
    }

    // Add or update a single node (incremental update that preserves physics)
    async function addNode(nodeData) {
        if (!nodesDataSet) return;

        const typeConfig = nodeTypes[nodeData.type] || nodeTypes.unknown;
        const status = nodeData.status || 'unverified';
        const truthStatus = nodeData.truth_status || '';

        // Truth status takes precedence for border color (matches renderNetwork)
        let borderColor;
        if (truthStatus === 'conflict') {
            borderColor = truthColors.conflict;  // Amber for conflicts
        } else if (truthStatus === 'asserted') {
            borderColor = truthColors.asserted;  // Gold for asserted truth
        } else {
            borderColor = statusColors[status] || statusColors.unverified;
        }

        // Tint icon based on status
        const iconColor = (status === 'verified' || status === 'degraded')
            ? typeConfig.color
            : theme.gray;
        const iconDataUri = await getTintedIcon(typeConfig.icon, iconColor);

        // Add truth indicator to label (matches renderNetwork)
        let label = (nodeData.label || nodeData.id).toUpperCase();
        if (truthStatus === 'asserted') {
            label = '\u2713 ' + label;  // Checkmark for truth
        } else if (truthStatus === 'conflict') {
            label = '\u26A0 ' + label;  // Warning for conflict
        }

        nodesDataSet.update({
            id: nodeData.id,
            label: label,
            title: buildTooltip(nodeData),
            shape: 'circularImage',
            image: iconDataUri,
            size: typeConfig.size,
            color: {
                border: borderColor,
                background: theme.offBlack,
                highlight: { border: theme.greenBright, background: theme.greenDarker },
                hover: { border: theme.greenBright, background: theme.greenDarker }
            },
            borderWidth: truthStatus ? 3 : 2,  // Thicker border for truth nodes
            font: { color: borderColor, face: 'VT323, monospace', size: 14, vadjust: 8 }
        });

        updateStats();
    }

    // Remove a node
    function removeNode(nodeId) {
        if (!nodesDataSet) return;
        nodesDataSet.remove(nodeId);
        updateStats();
    }

    // Add a single edge
    function addEdge(edgeData) {
        if (!edgesDataSet) return;

        edgesDataSet.update({
            id: edgeData.id || `${edgeData.from_id}-${edgeData.to_id}`,
            from: edgeData.from_id,
            to: edgeData.to_id,
            color: { color: theme.greenMedium, highlight: theme.greenBright, hover: theme.greenBright },
            width: 2
        });

        updateStats();
    }

    // Remove an edge
    function removeEdge(edgeId) {
        if (!edgesDataSet) return;
        edgesDataSet.remove(edgeId);
        updateStats();
    }

    // Update stats display
    function updateStats() {
        if (nodesDataSet) {
            elements.nodeCount.textContent = String(nodesDataSet.length).padStart(2, '0');
        }
        if (edgesDataSet) {
            elements.edgeCount.textContent = String(edgesDataSet.length).padStart(2, '0');
        }
    }

    // Connect to SSE for real-time updates
    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource('/events');

        eventSource.onopen = () => {
            console.log('SSE connected');
            updateStatus('OPERATIONAL');
        };

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                handleEvent(data);
            } catch (error) {
                console.error('Failed to parse SSE event:', error);
            }
        };

        eventSource.onerror = () => {
            console.log('SSE disconnected, reconnecting...');
            updateStatus('RECONNECTING');
            eventSource.close();
            setTimeout(connectSSE, 5000);
        };
    }

    // Discrepancy Panel Functions
    function toggleDiscrepancyPanel() {
        elements.discrepancyPanel.classList.toggle('collapsed');
    }

    function showDiscrepancyPanel() {
        elements.discrepancyPanel.classList.remove('hidden');
    }

    function hideDiscrepancyPanel() {
        elements.discrepancyPanel.classList.add('hidden');
    }

    function updateDiscrepancyBadge(count) {
        if (count > 0) {
            elements.discrepancyBadge.textContent = count;
            elements.discrepancyBadge.classList.add('active');
            showDiscrepancyPanel();
        } else {
            elements.discrepancyBadge.classList.remove('active');
        }
    }

    async function loadDiscrepancies() {
        try {
            const response = await fetch('/api/discrepancies');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            discrepancies = await response.json();
            renderDiscrepancies();
            updateDiscrepancyBadge(discrepancies.length);

        } catch (error) {
            console.error('Failed to load discrepancies:', error);
        }
    }

    function renderDiscrepancies() {
        if (!discrepancies || discrepancies.length === 0) {
            elements.discrepancyContent.innerHTML = '<div class="discrepancy-empty">No heresy detected. The Omnissiah is pleased.</div>';
            return;
        }

        let html = '';
        for (const d of discrepancies) {
            const detectedTime = new Date(d.detected_at).toLocaleString();
            html += `
                <div class="discrepancy-item" data-discrepancy-id="${d.id}" data-node-id="${d.node_id}">
                    <div class="discrepancy-item-header">
                        <span class="discrepancy-item-node">${d.node_id}</span>
                        <span class="discrepancy-item-property">${d.property_key}</span>
                    </div>
                    <div class="discrepancy-item-values">
                        <div class="discrepancy-value-row">
                            <span class="discrepancy-value-label">TRUTH:</span>
                            <span class="discrepancy-value-truth">${formatValue(d.truth_value)}</span>
                        </div>
                        <div class="discrepancy-value-row">
                            <span class="discrepancy-value-label">ACTUAL:</span>
                            <span class="discrepancy-value-actual">${formatValue(d.actual_value)}</span>
                        </div>
                    </div>
                    <div class="discrepancy-item-time">${detectedTime}</div>
                </div>
            `;
        }

        elements.discrepancyContent.innerHTML = html;

        // Add click handlers to open node detail modal
        elements.discrepancyContent.querySelectorAll('.discrepancy-item').forEach(item => {
            item.addEventListener('click', () => {
                const nodeId = item.getAttribute('data-node-id');
                openNodeDetailModal(nodeId);
            });
        });
    }

    function formatValue(value) {
        if (value === null || value === undefined) return '<empty>';
        if (typeof value === 'object') return JSON.stringify(value);
        return String(value);
    }

    // Node Detail Modal Functions
    async function openNodeDetailModal(nodeId) {
        try {
            // Fetch node data
            const response = await fetch(`/api/nodes/${nodeId}`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const node = await response.json();
            currentNodeId = nodeId;
            currentNodeData = node;

            // Update modal title
            elements.nodeDetailTitle.textContent = `> ${(node.label || node.id).toUpperCase()}`;

            // Render all modal sections
            renderStatusBar(node);
            renderExistenceSection(node);
            renderIdentitySection(node);
            renderNetworkSection(node);
            renderHostnameInferenceSection(node);
            renderTruthProperties(node);

            // Setup collapsible section toggle
            setupCollapsibleSections();

            // Show modal
            elements.nodeDetailModal.classList.add('active');

        } catch (error) {
            console.error('Failed to load node:', error);
            updateStatus('ERROR: ' + error.message);
        }
    }

    function closeNodeDetailModal() {
        elements.nodeDetailModal.classList.remove('active');
        currentNodeId = null;
        currentNodeData = null;
    }

    function renderStatusBar(node) {
        const status = node.status || 'unverified';
        const statusBar = document.getElementById('node-status-bar');

        let html = `
            <div class="status-item">
                <span class="status-label">STATUS</span>
                <span class="status-value ${status}">${status.toUpperCase()}</span>
            </div>
        `;

        if (node.last_verified) {
            const lastVerified = new Date(node.last_verified).toLocaleString();
            html += `
                <div class="status-item">
                    <span class="status-label">LAST VERIFIED</span>
                    <span class="status-value">${lastVerified}</span>
                </div>
            `;
        }

        if (node.truth_status) {
            const truthClass = node.truth_status === 'conflict' ? 'status-value degraded' : 'status-value highlight';
            html += `
                <div class="status-item">
                    <span class="status-label">TRUTH</span>
                    <span class="${truthClass}">${node.truth_status.toUpperCase()}</span>
                </div>
            `;
        }

        statusBar.innerHTML = html;
    }

    function renderExistenceSection(node) {
        // Get current existence value from truth
        let existenceValue = '';
        if (node.truth && node.truth.properties && node.truth.properties.existence) {
            existenceValue = node.truth.properties.existence;
        }

        // Set the appropriate radio button
        const radios = document.querySelectorAll('input[name="existence"]');
        radios.forEach(radio => {
            radio.checked = (radio.value === existenceValue);
        });
    }

    function renderIdentitySection(node) {
        const container = document.getElementById('node-identity-content');

        let html = '<div class="property-grid">';
        html += `
            <div class="property-item">
                <span class="property-label">ID</span>
                <span class="property-value">${node.id}</span>
            </div>
            <div class="property-item">
                <span class="property-label">TYPE</span>
                <span class="property-value">${node.type || 'unknown'}</span>
            </div>
            <div class="property-item">
                <span class="property-label">LABEL</span>
                <span class="property-value">${node.label || node.id}</span>
            </div>
        `;

        if (node.source) {
            html += `
                <div class="property-item">
                    <span class="property-label">SOURCE</span>
                    <span class="property-value">${node.source}</span>
                </div>
            `;
        }

        // Show parent for interface/satellite nodes
        if (node.parent_id) {
            html += `
                <div class="property-item">
                    <span class="property-label">PARENT</span>
                    <span class="property-value link" data-node-id="${node.parent_id}">${node.parent_id}</span>
                </div>
            `;
        }

        html += '</div>';

        // Add click handler for parent link after setting innerHTML
        container.innerHTML = html;

        // Setup click handlers for parent links
        container.querySelectorAll('.property-value.link').forEach(el => {
            el.style.cursor = 'pointer';
            el.style.textDecoration = 'underline';
            el.addEventListener('click', () => {
                const parentId = el.dataset.nodeId;
                if (parentId) {
                    openNodeDetailModal(parentId);
                }
            });
        });
    }

    function renderNetworkSection(node) {
        const container = document.getElementById('node-network-content');
        const props = node.properties || {};
        const discovered = node.discovered || {};

        let html = '<div class="property-grid">';

        // IP
        const ip = props.ip || discovered.ip;
        if (ip) {
            html += `
                <div class="property-item">
                    <span class="property-label">IP ADDRESS</span>
                    <span class="property-value">${ip}</span>
                </div>
            `;
        }

        // MAC
        const mac = discovered.mac_address || props.mac_address;
        if (mac) {
            html += `
                <div class="property-item">
                    <span class="property-label">MAC ADDRESS</span>
                    <span class="property-value">${mac}</span>
                </div>
            `;
        }

        // Reverse DNS
        if (discovered.reverse_dns) {
            html += `
                <div class="property-item">
                    <span class="property-label">REVERSE DNS</span>
                    <span class="property-value">${discovered.reverse_dns}</span>
                </div>
            `;
        }

        // Latency
        if (discovered.ping_latency_ms !== undefined) {
            html += `
                <div class="property-item">
                    <span class="property-label">LATENCY</span>
                    <span class="property-value">${discovered.ping_latency_ms}ms</span>
                </div>
            `;
        }

        // Open Ports
        if (discovered.open_ports && discovered.open_ports.length > 0) {
            html += `
                <div class="property-item">
                    <span class="property-label">OPEN PORTS</span>
                    <span class="property-value">${discovered.open_ports.join(', ')}</span>
                </div>
            `;
        }

        // Services
        if (discovered.services && discovered.services.length > 0) {
            const serviceList = discovered.services.map(s =>
                `${s.port}/${s.service}`
            ).join(', ');
            html += `
                <div class="property-item" style="grid-column: span 2;">
                    <span class="property-label">SERVICES</span>
                    <span class="property-value">${serviceList}</span>
                </div>
            `;
        }

        html += '</div>';

        if (html === '<div class="property-grid"></div>') {
            html = '<span class="property-value dim">No network data discovered</span>';
        }

        container.innerHTML = html;
    }

    function renderHostnameInferenceSection(node) {
        const section = document.getElementById('hostname-inference-section');
        const container = document.getElementById('hostname-inference-content');

        const inference = node.discovered?.hostname_inference;
        if (!inference || !inference.candidates || inference.candidates.length === 0) {
            section.style.display = 'none';
            return;
        }

        section.style.display = 'block';

        let html = '<div class="inference-candidates">';

        for (const candidate of inference.candidates) {
            const isBest = inference.best && candidate.hostname === inference.best.hostname;
            const confidencePercent = Math.round(candidate.confidence * 100);

            html += `
                <div class="inference-candidate ${isBest ? 'best' : ''}">
                    <span class="inference-hostname">${candidate.hostname}</span>
                    <span class="inference-confidence">${confidencePercent}%</span>
                    <span class="inference-source">${candidate.source}</span>
                </div>
            `;
        }

        html += '</div>';
        container.innerHTML = html;
    }

    function setupCollapsibleSections() {
        const toggle = document.getElementById('truth-section-toggle');
        if (toggle) {
            toggle.onclick = () => {
                const section = toggle.closest('.collapsible');
                if (section) {
                    section.classList.toggle('collapsed');
                }
            };
        }
    }

    // Truthable properties
    const truthableProperties = ['ip', 'hostname', 'mac_address', 'type', 'description', 'location', 'owner', 'expected_ports'];

    function renderTruthProperties(node) {
        let html = '';

        for (const prop of truthableProperties) {
            // Get current value from various sources
            let currentValue = '';
            let isLocked = false;
            let hasConflict = false;

            // Check truth first
            if (node.truth && node.truth.properties && node.truth.properties[prop] !== undefined) {
                currentValue = String(node.truth.properties[prop]);
                isLocked = true;
            }

            // Check discovered value
            let discoveredValue = '';
            if (node.discovered && node.discovered[prop] !== undefined) {
                discoveredValue = String(node.discovered[prop]);
            }

            // Check properties
            if (!currentValue && node.properties && node.properties[prop] !== undefined) {
                currentValue = String(node.properties[prop]);
            }

            // If locked and discovered differs, it's a conflict
            if (isLocked && discoveredValue && currentValue !== discoveredValue) {
                hasConflict = true;
            }

            // Determine status indicator
            let statusHtml = '';
            if (isLocked) {
                if (hasConflict) {
                    statusHtml = '<span class="truth-property-status conflict">!</span>';
                } else {
                    statusHtml = '<span class="truth-property-status match">\u2713</span>';
                }
            }

            const inputClass = isLocked ? 'truth-property-input locked' : 'truth-property-input';

            html += `
                <div class="truth-property-row">
                    <input type="checkbox" class="truth-property-checkbox" data-prop="${prop}" ${isLocked ? 'checked' : ''}>
                    <span class="truth-property-label">${prop.toUpperCase()}</span>
                    <input type="text" class="${inputClass}" data-prop="${prop}" value="${currentValue}" placeholder="${discoveredValue || 'Not set'}">
                    ${statusHtml}
                </div>
            `;
        }

        elements.truthProperties.innerHTML = html;

        // Add change handlers to checkboxes to toggle input style
        elements.truthProperties.querySelectorAll('.truth-property-checkbox').forEach(checkbox => {
            checkbox.addEventListener('change', () => {
                const prop = checkbox.getAttribute('data-prop');
                const input = elements.truthProperties.querySelector(`input.truth-property-input[data-prop="${prop}"]`);
                if (checkbox.checked) {
                    input.classList.add('locked');
                } else {
                    input.classList.remove('locked');
                }
            });
        });
    }

    async function handleSetTruth() {
        if (!currentNodeId) return;

        // Gather properties
        const properties = {};

        // Get existence from radio buttons
        const existenceRadio = document.querySelector('input[name="existence"]:checked');
        if (existenceRadio && existenceRadio.value) {
            properties.existence = existenceRadio.value;
        }

        // Gather checked truth properties
        elements.truthProperties.querySelectorAll('.truth-property-checkbox:checked').forEach(checkbox => {
            const prop = checkbox.getAttribute('data-prop');
            const input = elements.truthProperties.querySelector(`input.truth-property-input[data-prop="${prop}"]`);
            const value = input.value.trim();
            if (value) {
                properties[prop] = value;
            }
        });

        if (Object.keys(properties).length === 0) {
            updateStatus('ERROR: NO PROPERTIES SET');
            return;
        }

        try {
            elements.setTruthBtn.disabled = true;
            updateStatus('SAVING TRUTH');

            const response = await fetch(`/api/nodes/${currentNodeId}/truth`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ properties: properties })
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.details || error.error || `HTTP ${response.status}`);
            }

            updateStatus('TRUTH SAVED');
            closeNodeDetailModal();
            await loadGraph();
            await loadDiscrepancies();

        } catch (error) {
            console.error('Failed to set truth:', error);
            updateStatus('ERROR: ' + error.message);
        } finally {
            elements.setTruthBtn.disabled = false;
        }
    }

    async function handleClearTruth() {
        if (!currentNodeId) return;

        if (!confirm('Clear all truth assertions for this node?')) {
            return;
        }

        try {
            elements.clearTruthBtn.disabled = true;
            updateStatus('CLEARING TRUTH');

            const response = await fetch(`/api/nodes/${currentNodeId}/truth`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.details || error.error || `HTTP ${response.status}`);
            }

            updateStatus('TRUTH CLEARED');
            closeNodeDetailModal();
            await loadGraph();
            await loadDiscrepancies();

        } catch (error) {
            console.error('Failed to clear truth:', error);
            updateStatus('ERROR: ' + error.message);
        } finally {
            elements.clearTruthBtn.disabled = false;
        }
    }

    // Handle SSE events
    function handleEvent(event) {
        console.log('SSE event:', event.type, event.payload);

        switch (event.type) {
            case 'node-created':
                if (event.payload) addNode(event.payload);
                else loadGraph();
                break;

            case 'node-updated':
                if (event.payload) addNode(event.payload);
                else loadGraph();
                break;

            case 'node-deleted':
                if (event.payload && event.payload.id) removeNode(event.payload.id);
                else loadGraph();
                break;

            case 'edge-created':
                if (event.payload) addEdge(event.payload);
                else loadGraph();
                break;

            case 'edge-deleted':
                if (event.payload && event.payload.id) removeEdge(event.payload.id);
                else loadGraph();
                break;

            case 'graph-updated':
            case 'import-completed':
                loadGraph();
                break;

            // Discovery events
            case 'discovery-started':
                expandDiscoveryLog();
                if (event.payload) {
                    addDiscoveryEntry(event.payload);
                    updateStatus(`DISCOVERING ${event.payload.total || 0} NODES`);
                }
                break;

            case 'discovery-progress':
                if (event.payload) {
                    addDiscoveryEntry(event.payload);
                }
                break;

            case 'discovery-complete':
                if (event.payload) {
                    addDiscoveryEntry(event.payload);
                    updateStatus(`DISCOVERY COMPLETE: ${event.payload.verified || 0} VERIFIED`);
                }
                // Don't reload graph - individual node-updated events already updated the UI
                // loadGraph() would reset physics positions
                break;

            // Truth events
            case 'truth-set':
                loadGraph();
                loadDiscrepancies();
                break;

            case 'truth-cleared':
                loadGraph();
                loadDiscrepancies();
                break;

            // Discrepancy events
            case 'discrepancy-created':
                loadDiscrepancies();
                updateStatus(`HERESY DETECTED: ${event.payload?.node_id || 'node'} - ${event.payload?.property || 'property'}`);
                break;

            case 'discrepancy-resolved':
                loadDiscrepancies();
                break;
        }
    }

    // Update status display
    function updateStatus(status) {
        elements.status.textContent = `> STATUS: ${status}`;
    }

    // ==================== MULTI-SELECT & MERGE ====================

    // Update selection UI (toolbar visibility, node highlighting)
    function updateSelectionUI() {
        const count = selectedNodes.size;
        elements.selectionCountValue.textContent = count;

        if (count >= 2) {
            elements.selectionToolbar.classList.remove('hidden');
        } else {
            elements.selectionToolbar.classList.add('hidden');
        }

        // Update vis-network selection to show visual highlight
        if (network) {
            network.selectNodes(Array.from(selectedNodes));
        }
    }

    // Clear all selected nodes
    function clearSelection() {
        selectedNodes.clear();
        updateSelectionUI();
        if (network) {
            network.unselectAll();
        }
    }

    // Open merge modal
    function openMergeModal() {
        if (selectedNodes.size < 2) {
            updateStatus('ERROR: Select at least 2 nodes to merge');
            return;
        }

        // Update count display
        elements.mergeSelectedCount.textContent = selectedNodes.size;

        // Try to infer a good default parent name from selected nodes
        const selectedIds = Array.from(selectedNodes);
        let suggestedName = '';

        // Check if any selected node has a hostname in properties
        for (const nodeId of selectedIds) {
            const nodeData = nodesDataSet.get(nodeId);
            if (nodeData && nodeData.title) {
                // Try to extract hostname from tooltip or use label
                const label = nodeData.label || nodeId;
                // Remove common suffixes and take the first word
                const cleaned = label.toLowerCase()
                    .replace(/[^a-z0-9-]/g, '')
                    .split('-')[0];
                if (cleaned.length > 2 && !cleaned.match(/^\d/)) {
                    suggestedName = cleaned;
                    break;
                }
            }
        }

        elements.mergeParentName.value = suggestedName;
        updateMergePreview();

        elements.mergeModal.classList.add('active');
    }

    // Close merge modal
    function closeMergeModal() {
        elements.mergeModal.classList.remove('active');
    }

    // Update merge preview based on current input
    function updateMergePreview() {
        const parentName = elements.mergeParentName.value.trim() || 'parent';
        const selectedIds = Array.from(selectedNodes).sort();

        let html = '<div class="merge-preview-title">PREVIEW</div>';

        selectedIds.forEach((nodeId, i) => {
            const nodeData = nodesDataSet.get(nodeId);
            const label = nodeData ? (nodeData.label || nodeId) : nodeId;
            const interfaceName = `eth${i}`;

            html += `
                <div class="merge-preview-item">
                    <span class="merge-preview-from">${label}</span>
                    <span class="merge-preview-arrow">→</span>
                    <span class="merge-preview-to">${parentName}:${interfaceName}</span>
                </div>
            `;
        });

        elements.mergePreview.innerHTML = html;
    }

    // Handle merge submission
    async function handleMerge() {
        const parentName = elements.mergeParentName.value.trim();
        const parentType = elements.mergeParentType.value;

        if (!parentName) {
            updateStatus('ERROR: Parent name is required');
            return;
        }

        if (selectedNodes.size < 2) {
            updateStatus('ERROR: At least 2 nodes required');
            return;
        }

        const nodeIds = Array.from(selectedNodes);

        try {
            updateStatus('MERGING...');

            const response = await fetch('/api/nodes/merge', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    node_ids: nodeIds,
                    parent_id: parentName,
                    parent_type: parentType
                })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            const result = await response.json();
            updateStatus(`MERGED: Created ${result.parent_id} with ${result.interface_count} interfaces`);

            // Close modal and clear selection
            closeMergeModal();
            clearSelection();

            // Reload graph to show changes
            await loadGraph();

        } catch (error) {
            console.error('Merge failed:', error);
            updateStatus('ERROR: ' + error.message);
        }
    }

    // ==================== SECRETS MANAGEMENT ====================

    // Secret type field definitions
    const secretTypeFields = {
        ssh_key: [
            { key: 'key_path', label: 'Key Path', type: 'text', placeholder: '/path/to/key' },
            { key: 'passphrase', label: 'Passphrase', type: 'password', placeholder: 'Optional passphrase' },
            { key: 'username', label: 'Username', type: 'text', placeholder: 'SSH username' }
        ],
        snmp_community: [
            { key: 'community', label: 'Community String', type: 'password', placeholder: 'e.g., public' }
        ],
        snmpv3: [
            { key: 'username', label: 'Username', type: 'text' },
            { key: 'auth_protocol', label: 'Auth Protocol', type: 'select', options: ['MD5', 'SHA', 'SHA256'] },
            { key: 'auth_password', label: 'Auth Password', type: 'password' },
            { key: 'priv_protocol', label: 'Privacy Protocol', type: 'select', options: ['DES', 'AES', 'AES256'] },
            { key: 'priv_password', label: 'Privacy Password', type: 'password' }
        ],
        api_token: [
            { key: 'token', label: 'API Token', type: 'password', placeholder: 'Bearer token or API key' },
            { key: 'base_url', label: 'Base URL', type: 'text', placeholder: 'https://api.example.com' }
        ],
        dns: [
            { key: 'server', label: 'DNS Server', type: 'text', placeholder: '192.168.0.5' }
        ],
        generic: [
            { key: 'value', label: 'Value', type: 'textarea', placeholder: 'Secret value' }
        ]
    };

    // Secret type icons (SVG paths)
    const secretTypeIcons = {
        ssh_key: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>',
        snmp_community: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="2"/><path d="M7 7h10M7 12h10M7 17h6"/></svg>',
        snmpv3: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="2"/><path d="M7 7h10M7 12h10M7 17h6"/><circle cx="18" cy="17" r="2"/></svg>',
        api_token: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>',
        dns: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
        generic: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>'
    };

    // Current edit state
    let currentEditSecret = null;

    // Open secrets management modal
    function openSecretsModal() {
        closeDropdown();
        elements.secretsModal.classList.add('active');
        loadSecrets();
    }

    // Close secrets management modal
    function closeSecretsModal() {
        elements.secretsModal.classList.remove('active');
    }

    // Load secrets from API
    async function loadSecrets() {
        const typeFilter = elements.secretsTypeFilter.value;
        const sourceFilter = elements.secretsSourceFilter.value;

        try {
            let url = '/api/secrets';
            const params = new URLSearchParams();
            if (typeFilter) params.append('type', typeFilter);
            if (sourceFilter) params.append('source', sourceFilter);
            if (params.toString()) url += '?' + params.toString();

            const response = await fetch(url);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const secrets = await response.json();
            renderSecretsList(secrets);

        } catch (error) {
            console.error('Failed to load secrets:', error);
            elements.secretsList.innerHTML = '<div class="secrets-empty">Failed to load secrets</div>';
        }
    }

    // Render secrets list
    function renderSecretsList(secrets) {
        if (!secrets || secrets.length === 0) {
            elements.secretsList.innerHTML = '<div class="secrets-empty">No secrets configured</div>';
            return;
        }

        let html = '';
        for (const secret of secrets) {
            const icon = secretTypeIcons[secret.type] || secretTypeIcons.generic;
            const isImmutable = secret.immutable || secret.source === 'mounted';

            html += `
                <div class="secret-item secret-type-${secret.type} ${isImmutable ? 'immutable' : ''}" data-secret-id="${secret.id}">
                    <div class="secret-icon">${icon}</div>
                    <div class="secret-details">
                        <div class="secret-header">
                            <span class="secret-name">${escapeHtml(secret.name)}</span>
                            <span class="secret-id">${escapeHtml(secret.id)}</span>
                        </div>
                        <div class="secret-badges">
                            <span class="secret-badge secret-badge-type">${secret.type}</span>
                            ${isImmutable ? '<span class="secret-badge secret-badge-mounted">MOUNTED</span>' : '<span class="secret-badge secret-badge-source">OPERATOR</span>'}
                            ${secret.status && secret.status !== 'unknown' ? `<span class="secret-badge secret-badge-status ${secret.status}">${secret.status}</span>` : ''}
                        </div>
                        ${secret.description ? `<div class="secret-description">${escapeHtml(secret.description)}</div>` : ''}
                        <div class="secret-meta">
                            ${secret.created_at ? `<span>Created: ${new Date(secret.created_at).toLocaleDateString()}</span>` : ''}
                            ${secret.last_used_at ? `<span>Last used: ${new Date(secret.last_used_at).toLocaleDateString()}</span>` : ''}
                        </div>
                    </div>
                    <div class="secret-actions">
                        ${!isImmutable ? `
                            <button class="secret-action-btn" onclick="editSecret('${secret.id}')">EDIT</button>
                            <button class="secret-action-btn danger" onclick="deleteSecret('${secret.id}')">DELETE</button>
                        ` : ''}
                    </div>
                </div>
            `;
        }

        elements.secretsList.innerHTML = html;
    }

    // HTML escape utility
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Open secret edit modal (for new or existing)
    function openSecretEditModal(secretData = null) {
        currentEditSecret = secretData;

        if (secretData) {
            elements.secretEditTitle.textContent = '> EDIT SECRET';
            elements.secretEditId.value = secretData.id;
            elements.secretEditId.disabled = true;
            elements.secretEditName.value = secretData.name || '';
            elements.secretEditType.value = secretData.type || 'generic';
            elements.secretEditType.disabled = true;
            elements.secretEditDescription.value = secretData.description || '';
        } else {
            elements.secretEditTitle.textContent = '> ADD SECRET';
            elements.secretEditId.value = '';
            elements.secretEditId.disabled = false;
            elements.secretEditName.value = '';
            elements.secretEditType.value = 'ssh_key';
            elements.secretEditType.disabled = false;
            elements.secretEditDescription.value = '';
        }

        updateSecretDataFields();
        elements.secretEditModal.classList.add('active');
    }

    // Close secret edit modal
    function closeSecretEditModal() {
        elements.secretEditModal.classList.remove('active');
        currentEditSecret = null;
    }

    // Update data fields based on selected type
    function updateSecretDataFields() {
        const type = elements.secretEditType.value;
        const fields = secretTypeFields[type] || secretTypeFields.generic;

        let html = '<div class="secret-data-fields"><div class="secret-data-fields-title">SECRET DATA</div>';

        for (const field of fields) {
            const currentValue = currentEditSecret?.data?.[field.key] || '';

            if (field.type === 'textarea') {
                html += `
                    <div class="form-group">
                        <label class="form-label">${field.label}</label>
                        <textarea class="form-input secret-field-textarea" data-key="${field.key}" placeholder="${field.placeholder || ''}">${escapeHtml(currentValue)}</textarea>
                    </div>
                `;
            } else if (field.type === 'select') {
                html += `
                    <div class="form-group">
                        <label class="form-label">${field.label}</label>
                        <select class="form-select" data-key="${field.key}">
                            ${field.options.map(opt => `<option value="${opt}" ${currentValue === opt ? 'selected' : ''}>${opt}</option>`).join('')}
                        </select>
                    </div>
                `;
            } else {
                html += `
                    <div class="form-group">
                        <label class="form-label">${field.label}</label>
                        <input type="${field.type}" class="form-input" data-key="${field.key}" value="${escapeHtml(currentValue)}" placeholder="${field.placeholder || ''}">
                    </div>
                `;
            }
        }

        html += '</div>';
        elements.secretEditDataFields.innerHTML = html;
    }

    // Handle secret save
    async function handleSecretSave() {
        const id = elements.secretEditId.value.trim();
        const name = elements.secretEditName.value.trim();
        const type = elements.secretEditType.value;
        const description = elements.secretEditDescription.value.trim();

        if (!id || !name) {
            updateStatus('ERROR: ID and name required');
            return;
        }

        // Gather data fields
        const data = {};
        elements.secretEditDataFields.querySelectorAll('[data-key]').forEach(el => {
            const key = el.dataset.key;
            const value = el.value.trim();
            if (value) {
                data[key] = value;
            }
        });

        const payload = {
            id: id,
            name: name,
            type: type,
            description: description,
            data: data
        };

        try {
            elements.secretEditSubmit.disabled = true;
            updateStatus(currentEditSecret ? 'UPDATING SECRET' : 'CREATING SECRET');

            const url = currentEditSecret ? `/api/secrets/${id}` : '/api/secrets';
            const method = currentEditSecret ? 'PUT' : 'POST';

            const response = await fetch(url, {
                method: method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.details || error.error || `HTTP ${response.status}`);
            }

            updateStatus(currentEditSecret ? 'SECRET UPDATED' : 'SECRET CREATED');
            closeSecretEditModal();
            loadSecrets();

        } catch (error) {
            console.error('Failed to save secret:', error);
            updateStatus('ERROR: ' + error.message);
        } finally {
            elements.secretEditSubmit.disabled = false;
        }
    }

    // Edit existing secret - exposed to global scope for onclick
    window.editSecret = async function(secretId) {
        try {
            const response = await fetch(`/api/secrets/${secretId}?include_data=true`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const secret = await response.json();
            openSecretEditModal(secret);
        } catch (error) {
            console.error('Failed to load secret:', error);
            updateStatus('ERROR: ' + error.message);
        }
    };

    // Delete secret - exposed to global scope for onclick
    window.deleteSecret = async function(secretId) {
        if (!confirm(`Delete secret "${secretId}"? This cannot be undone.`)) {
            return;
        }

        try {
            updateStatus('DELETING SECRET');
            const response = await fetch(`/api/secrets/${secretId}`, { method: 'DELETE' });
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.details || error.error || `HTTP ${response.status}`);
            }
            updateStatus('SECRET DELETED');
            loadSecrets();
        } catch (error) {
            console.error('Failed to delete secret:', error);
            updateStatus('ERROR: ' + error.message);
        }
    };

    // ========================================================================
    // SEGMENTUM (Subnet Nebula) Rendering
    // Draws galactic-style nebulous regions behind nodes grouped by subnet
    // ========================================================================

    // Get or assign a color for a segmentum (subnet CIDR)
    function getSegmentumColor(cidr) {
        if (!segmentumColors[cidr]) {
            segmentumColors[cidr] = segmentumPalette[segmentumColorIndex % segmentumPalette.length];
            segmentumColorIndex++;
        }
        return segmentumColors[cidr];
    }

    // Get segmentum (subnet) for a node - from properties or inferred from IP
    function getNodeSegmentum(nodeData) {
        // First check explicit segmentum property
        if (nodeData._segmentum) return nodeData._segmentum;
        if (nodeData.properties?.segmentum) return nodeData.properties.segmentum;

        // Fallback: infer from IP address (assume /24)
        const ip = nodeData._ip || nodeData.properties?.ip;
        if (ip && typeof ip === 'string') {
            const parts = ip.split('.');
            if (parts.length === 4) {
                return `${parts[0]}.${parts[1]}.${parts[2]}.0/24`;
            }
        }
        return null;
    }

    // Draw all segmenta (subnet nebulae) on the canvas
    function drawSegmenta(ctx) {
        if (!network || !nodesDataSet) return;

        // Group nodes by segmentum
        const nodesBySegmentum = {};
        const allNodeIds = nodesDataSet.getIds();

        allNodeIds.forEach(nodeId => {
            const nodeData = nodesDataSet.get(nodeId);
            if (!nodeData) return;

            // Skip interface/satellite nodes - they follow their parent
            if (nodeData._isSatellite) return;

            const segmentum = getNodeSegmentum(nodeData);
            if (!segmentum) return;

            if (!nodesBySegmentum[segmentum]) {
                nodesBySegmentum[segmentum] = [];
            }
            nodesBySegmentum[segmentum].push(nodeId);
        });

        // Get all node positions
        const positions = network.getPositions();

        // Draw each segmentum as a nebulous cloud
        Object.entries(nodesBySegmentum).forEach(([cidr, nodeIds]) => {
            if (nodeIds.length < 1) return;

            // Calculate bounding box with padding
            const nodePositions = nodeIds
                .map(id => positions[id])
                .filter(p => p);

            if (nodePositions.length === 0) return;

            const xs = nodePositions.map(p => p.x);
            const ys = nodePositions.map(p => p.y);

            const minX = Math.min(...xs);
            const maxX = Math.max(...xs);
            const minY = Math.min(...ys);
            const maxY = Math.max(...ys);

            // Add generous padding for the nebula effect
            const padding = 120;
            const centerX = (minX + maxX) / 2;
            const centerY = (minY + maxY) / 2;
            const width = Math.max(maxX - minX + padding * 2, 200);
            const height = Math.max(maxY - minY + padding * 2, 200);

            // Get color for this segmentum
            const color = getSegmentumColor(cidr);

            // Draw nebulous cloud using multiple radial gradients
            drawNebula(ctx, centerX, centerY, width, height, color, nodePositions);

            // Draw segmentum label
            drawSegmentumLabel(ctx, cidr, centerX, minY - padding + 20);
        });
    }

    // Draw a nebulous cloud effect
    function drawNebula(ctx, centerX, centerY, width, height, color, nodePositions) {
        ctx.save();

        // Main elliptical gradient for the overall shape
        const radiusX = width / 2;
        const radiusY = height / 2;

        // Create radial gradient from center
        const gradient = ctx.createRadialGradient(
            centerX, centerY, 0,
            centerX, centerY, Math.max(radiusX, radiusY)
        );
        gradient.addColorStop(0, color.glow);
        gradient.addColorStop(0.4, color.base);
        gradient.addColorStop(1, 'rgba(0, 0, 0, 0)');

        // Draw main ellipse
        ctx.beginPath();
        ctx.ellipse(centerX, centerY, radiusX * 1.2, radiusY * 1.2, 0, 0, Math.PI * 2);
        ctx.fillStyle = gradient;
        ctx.fill();

        // Add smaller "wisps" around each node for organic feel
        nodePositions.forEach((pos, i) => {
            const wispRadius = 60 + (i % 3) * 20; // Vary size
            const wispGradient = ctx.createRadialGradient(
                pos.x, pos.y, 0,
                pos.x, pos.y, wispRadius
            );
            wispGradient.addColorStop(0, color.glow);
            wispGradient.addColorStop(0.5, color.base);
            wispGradient.addColorStop(1, 'rgba(0, 0, 0, 0)');

            ctx.beginPath();
            ctx.arc(pos.x, pos.y, wispRadius, 0, Math.PI * 2);
            ctx.fillStyle = wispGradient;
            ctx.fill();
        });

        ctx.restore();
    }

    // Draw segmentum label
    function drawSegmentumLabel(ctx, cidr, x, y) {
        ctx.save();

        ctx.font = '14px VT323, monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';

        // Format CIDR as SEGMENTUM designation
        const label = `SEGMENTUM ${cidr}`;

        // Draw with glow effect
        ctx.shadowColor = 'rgba(57, 255, 20, 0.5)';
        ctx.shadowBlur = 8;
        ctx.fillStyle = 'rgba(57, 255, 20, 0.6)';
        ctx.fillText(label, x, y);

        ctx.restore();
    }

    // Start
    document.addEventListener('DOMContentLoaded', init);
})();
