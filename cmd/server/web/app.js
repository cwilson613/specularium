(function() {
    'use strict';

    // State
    let network = null;
    let nodesDataSet = null;
    let edgesDataSet = null;
    let eventSource = null;

    // Theme colors (matching CSS variables)
    const theme = {
        greenBright: '#39ff14',
        greenMedium: '#32cd32',
        greenDim: '#228b22',
        greenDark: '#1a5c1a',
        greenDarker: '#0d2d0d',
        offBlack: '#0a0a0a',
        connection1g: '#74c0fc',
        connection2_5g: '#69db7c',
        connection10g: '#ffa94d',
        connection25g: '#ff6b6b'
    };

    // DOM elements
    const elements = {
        container: document.getElementById('network-container'),
        loading: document.getElementById('loading'),
        nodeCount: document.getElementById('node-count'),
        edgeCount: document.getElementById('edge-count'),
        clientCount: document.getElementById('client-count'),
        status: document.getElementById('status'),
        refreshBtn: document.getElementById('refresh-btn')
    };

    // Initialize
    async function init() {
        elements.refreshBtn.addEventListener('click', loadGraph);
        await loadGraph();
        connectSSE();
    }

    // Load graph data
    async function loadGraph() {
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
            updateStatus('ERROR: ' + error.message);
        }
    }

    // Render network graph
    function renderNetwork(graph) {
        // Update stats
        elements.nodeCount.textContent = String(graph.nodes.length).padStart(2, '0');
        elements.edgeCount.textContent = String(graph.edges.length).padStart(2, '0');

        // Transform nodes
        const nodes = graph.nodes.map(n => ({
            id: n.id,
            label: n.label.toUpperCase(),
            title: n.title,
            group: n.group,
            x: n.position?.x,
            y: n.position?.y,
            fixed: n.position ? { x: true, y: true } : false,
            shape: 'box',
            font: {
                size: 18,
                color: theme.greenBright,
                face: 'VT323, monospace'
            },
            color: {
                border: theme.greenMedium,
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
            borderWidth: 2,
            borderWidthSelected: 3,
            margin: { top: 12, right: 18, bottom: 12, left: 18 },
            shapeProperties: {
                borderRadius: 0,
                borderDashes: n.group === 'device' ? [5, 3] : false
            }
        }));

        // Transform edges
        const edges = graph.edges.map(e => ({
            id: e.id,
            from: e.from,
            to: e.to,
            label: e.label,
            font: {
                size: 13,
                color: theme.greenBright,
                face: 'VT323, monospace',
                align: 'top',
                background: 'rgba(0, 0, 0, 0.7)',
                strokeWidth: 0
            },
            color: {
                color: getEdgeColor(e.speed_gbps),
                highlight: theme.greenBright,
                hover: theme.greenBright,
                inherit: false,
                opacity: 1.0
            },
            width: 4,
            smooth: {
                enabled: true,
                type: 'continuous',
                roundness: 0.5
            }
        }));

        // Create or update DataSets
        if (nodesDataSet) {
            nodesDataSet.clear();
            nodesDataSet.add(nodes);
        } else {
            nodesDataSet = new vis.DataSet(nodes);
        }

        if (edgesDataSet) {
            edgesDataSet.clear();
            edgesDataSet.add(edges);
        } else {
            edgesDataSet = new vis.DataSet(edges);
        }

        // Create network if not exists
        if (!network) {
            const options = {
                nodes: {
                    font: {
                        color: theme.greenBright,
                        size: 16,
                        face: 'VT323, monospace'
                    },
                    borderWidth: 2,
                    margin: { top: 12, right: 16, bottom: 12, left: 16 },
                    shape: 'box',
                    shapeProperties: { borderRadius: 0 },
                    color: {
                        border: theme.greenMedium,
                        background: theme.offBlack,
                        highlight: {
                            border: theme.greenBright,
                            background: theme.greenDarker
                        },
                        hover: {
                            border: theme.greenBright,
                            background: theme.greenDarker
                        }
                    }
                },
                edges: {
                    font: {
                        color: theme.greenBright,
                        size: 13,
                        strokeWidth: 0,
                        face: 'VT323, monospace',
                        align: 'top',
                        background: 'rgba(0, 0, 0, 0.7)'
                    },
                    arrows: { to: false },
                    smooth: {
                        enabled: true,
                        type: 'continuous',
                        roundness: 0.5
                    },
                    width: 4,
                    color: {
                        color: theme.greenMedium,
                        highlight: theme.greenBright,
                        hover: theme.greenBright,
                        inherit: false
                    }
                },
                physics: {
                    enabled: true,
                    stabilization: {
                        enabled: true,
                        iterations: 300,
                        updateInterval: 25,
                        fit: true
                    },
                    barnesHut: {
                        gravitationalConstant: -18000,
                        centralGravity: 0.3,
                        springLength: 350,
                        springConstant: 0.04,
                        damping: 0.2,
                        avoidOverlap: 0.8
                    }
                },
                interaction: {
                    dragNodes: true,
                    dragView: true,
                    zoomView: true,
                    hover: true,
                    tooltipDelay: 300,
                    keyboard: { enabled: true, bindToWindow: false }
                },
                layout: {
                    improvedLayout: true,
                    hierarchical: false
                },
                autoResize: true
            };

            network = new vis.Network(
                elements.container,
                { nodes: nodesDataSet, edges: edgesDataSet },
                options
            );

            // Event handlers
            network.on('stabilizationIterationsDone', () => {
                network.setOptions({
                    physics: { enabled: true, stabilization: false }
                });
                network.fit({
                    animation: { duration: 1000, easingFunction: 'easeInOutQuad' }
                });
            });

            network.on('dragEnd', (params) => {
                if (params.nodes.length > 0) {
                    savePositions(params.nodes);
                }
            });

            // Hover highlighting
            network.on('hoverNode', (params) => {
                highlightConnected(params.node, true);
            });

            network.on('blurNode', () => {
                highlightConnected(null, false);
            });
        }
    }

    // Get edge color based on speed
    function getEdgeColor(speedGbps) {
        if (speedGbps >= 25) return theme.connection25g;
        if (speedGbps >= 10) return theme.connection10g;
        if (speedGbps >= 2.5) return theme.connection2_5g;
        return theme.connection1g;
    }

    // Highlight connected nodes and edges
    function highlightConnected(nodeId, highlight) {
        if (!network || !nodesDataSet || !edgesDataSet) return;

        const allNodes = nodesDataSet.get();
        const allEdges = edgesDataSet.get();

        if (!highlight) {
            // Reset all
            nodesDataSet.update(allNodes.map(n => ({
                id: n.id,
                opacity: 1,
                borderWidth: 2,
                color: {
                    border: theme.greenMedium,
                    background: theme.offBlack
                },
                font: { color: theme.greenBright }
            })));

            edgesDataSet.update(allEdges.map(e => ({
                id: e.id,
                opacity: 1,
                width: 4
            })));
            return;
        }

        const connectedNodes = network.getConnectedNodes(nodeId);
        const connectedEdges = network.getConnectedEdges(nodeId);

        // Update nodes
        nodesDataSet.update(allNodes.map(n => {
            if (n.id === nodeId) {
                return {
                    id: n.id,
                    borderWidth: 3,
                    color: { border: theme.greenBright },
                    font: { color: theme.greenBright }
                };
            } else if (connectedNodes.includes(n.id)) {
                return {
                    id: n.id,
                    opacity: 1,
                    borderWidth: 3,
                    color: { border: theme.greenMedium },
                    font: { color: theme.greenMedium }
                };
            } else {
                return {
                    id: n.id,
                    opacity: 0.3,
                    color: { border: theme.greenDarker },
                    font: { color: theme.greenDark }
                };
            }
        }));

        // Update edges
        edgesDataSet.update(allEdges.map(e => {
            if (connectedEdges.includes(e.id)) {
                return { id: e.id, opacity: 1, width: 6 };
            } else {
                return { id: e.id, opacity: 0.3, width: 3 };
            }
        }));
    }

    // Save node positions
    async function savePositions(nodeIds) {
        const positions = network.getPositions(nodeIds);

        try {
            const response = await fetch('/api/positions', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(positions)
            });

            if (!response.ok) {
                console.error('Failed to save positions:', response.status);
            }
        } catch (error) {
            console.error('Failed to save positions:', error);
        }
    }

    // Connect to SSE for real-time updates
    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource('/api/events');

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

            // Reconnect after delay
            setTimeout(connectSSE, 5000);
        };
    }

    // Handle SSE events
    function handleEvent(event) {
        console.log('SSE event:', event.type);

        switch (event.type) {
            case 'host_created':
            case 'host_updated':
            case 'host_deleted':
            case 'connection_created':
            case 'connection_deleted':
            case 'infrastructure_reloaded':
                // Reload the entire graph
                loadGraph();
                break;

            case 'positions_updated':
                // Update positions without full reload
                if (event.payload && nodesDataSet) {
                    const updates = Object.entries(event.payload).map(([id, pos]) => ({
                        id,
                        x: pos.x,
                        y: pos.y,
                        fixed: { x: true, y: true }
                    }));
                    nodesDataSet.update(updates);
                }
                break;
        }
    }

    // Update status display
    function updateStatus(status) {
        elements.status.textContent = `> STATUS: ${status}`;
    }

    // Start
    document.addEventListener('DOMContentLoaded', init);
})();
