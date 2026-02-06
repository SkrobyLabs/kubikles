import React, { createContext, useContext, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { EventsOn, EventsOff } from 'wailsjs/runtime/runtime';
import { getClientId } from '../lib/wailsjs-adapter/runtime/runtime';
import { CheckAIProvider, StartAISession, SendAIMessage, CancelAIRequest, ClearAISession, CloseAISession } from 'wailsjs/go/main/App';
import { useK8s } from './K8sContext';
import { useUI } from './UIContext';
import { useConfig } from './ConfigContext';

// ===========================
// Type Definitions
// ===========================

interface AIProviderCheckResult {
    available: boolean;
    status: string;
    provider?: string;
}

interface TokenUsage {
    inputTokens?: number;
    outputTokens?: number;
    totalTokens?: number;
}

interface AIResponseEvent {
    sessionId?: string;
    generation?: number;
    error?: string;
    chunk?: string;
    usage?: TokenUsage;
    done?: boolean;
}

type MessageRole = 'user' | 'assistant' | 'thought';

interface ChatMessage {
    id: string;
    role: MessageRole;
    content: string;
    streaming: boolean;
    isError: boolean;
    usage?: TokenUsage;
}

interface ConversationHistoryItem {
    id: string;
    title: string;
    messages: ChatMessage[];
    updatedAt: number;
}

interface ParsedTabId {
    action: string;
    resourceType: string;
}

interface AIChatContextValue {
    isOpen: boolean;
    togglePanel: () => void;
    messages: ChatMessage[];
    sendMessage: (text: string) => void;
    isStreaming: boolean;
    cancelRequest: () => void;
    startNewChat: () => void;
    loadConversation: (convId: string) => void;
    deleteConversation: (convId: string) => void;
    conversationHistory: ConversationHistoryItem[];
    conversationId: string | null;
    providerAvailable: boolean | null;
    providerStatus: string;
    providerName: string;
    autoExecutedNavRef: React.MutableRefObject<Set<string>>;
}

// Parse tab ID to extract resource type and action
// Format: "details-pods-<uid>", "yaml-deployments-<uid>", "logs-pods-<uid>", "deps-services-<uid>"
function parseTabId(tabId: string | undefined): ParsedTabId | null {
    if (!tabId) return null;
    const parts = tabId.split('-');
    if (parts.length < 3) return null;
    return { action: parts[0], resourceType: parts[1] };
}

// Maps view names to the most relevant MCP tools for that view.
// The AI uses this to know which tools are useful given the user's current screen.
const VIEW_TOOL_MAP: Record<string, string[]> = {
    'metrics-overview':    ['get_cluster_metrics', 'get_pod_metrics'],
    'nodes':               ['get_cluster_metrics', 'list_resources', 'describe_resource'],
    'namespaces':          ['get_namespace_summary', 'list_resources'],
    'events':              ['get_events', 'get_flow_timeline'],
    'pods':                ['get_pod_metrics', 'list_resources', 'get_pod_logs', 'describe_resource', 'get_flow_timeline', 'get_multi_pod_logs'],
    'deployments':         ['list_resources', 'describe_resource', 'get_resource_dependencies', 'get_flow_timeline', 'get_multi_pod_logs', 'diff_resources'],
    'statefulsets':        ['list_resources', 'describe_resource', 'get_resource_dependencies', 'get_flow_timeline', 'get_multi_pod_logs', 'diff_resources'],
    'daemonsets':          ['list_resources', 'describe_resource', 'get_resource_dependencies', 'get_flow_timeline', 'get_multi_pod_logs'],
    'replicasets':         ['list_resources', 'describe_resource'],
    'jobs':                ['list_resources', 'describe_resource', 'get_pod_logs', 'get_flow_timeline'],
    'cronjobs':            ['list_resources', 'describe_resource'],
    'configmaps':          ['list_resources', 'get_resource_yaml', 'diff_resources'],
    'secrets':             ['list_resources', 'get_resource_yaml', 'diff_resources'],
    'hpas':                ['list_resources', 'describe_resource'],
    'services':            ['list_resources', 'describe_resource', 'get_resource_dependencies', 'diff_resources'],
    'ingresses':           ['list_resources', 'describe_resource', 'get_resource_dependencies'],
    'pvcs':                ['list_resources', 'describe_resource', 'get_resource_dependencies'],
    'pvs':                 ['list_resources', 'describe_resource'],
    'serviceaccounts':     ['list_resources', 'describe_resource', 'check_rbac_access'],
    'roles':               ['list_resources', 'get_resource_yaml'],
    'clusterroles':        ['list_resources', 'get_resource_yaml'],
    'rolebindings':        ['list_resources', 'get_resource_yaml'],
    'clusterrolebindings': ['list_resources', 'get_resource_yaml'],
    'crds':                ['list_crds', 'list_custom_resources'],
    // Diagnostic views
    'flow-timeline':       ['get_flow_timeline', 'get_events', 'describe_resource'],
    'multi-log-viewer':    ['get_multi_pod_logs', 'get_pod_logs', 'list_resources'],
    'resource-diff':       ['diff_resources', 'get_resource_yaml', 'describe_resource'],
    'rbac-checker':        ['check_rbac_access', 'list_resources', 'describe_resource'],
};

const TAB_ACTION_TOOL_MAP: Record<string, string[]> = {
    'details': ['describe_resource', 'get_resource_dependencies', 'get_events', 'get_flow_timeline'],
    'yaml':    ['get_resource_yaml', 'diff_resources'],
    'logs':    ['get_pod_logs', 'get_multi_pod_logs'],
    'deps':    ['get_resource_dependencies'],
};

const AIChatContext = createContext<AIChatContextValue | undefined>(undefined);

const HISTORY_STORAGE_KEY = 'kubikles-ai-chat-history';
const MAX_CONVERSATIONS = 10;

// Load conversation history from localStorage
function loadConversationHistory(): ConversationHistoryItem[] {
    try {
        const data = localStorage.getItem(HISTORY_STORAGE_KEY);
        return data ? JSON.parse(data) : [];
    } catch {
        return [];
    }
}

// Save conversation history to localStorage
function saveConversationHistory(conversations: ConversationHistoryItem[]): void {
    try {
        localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(conversations));
    } catch (e) {
        console.error('Failed to save conversation history:', e);
    }
}

// Generate a title from the first user message
function generateTitle(messages: ChatMessage[]): string {
    const firstUserMsg = messages.find(m => m.role === 'user');
    if (!firstUserMsg) return 'New conversation';
    const text = firstUserMsg.content;
    return text.length > 50 ? text.slice(0, 50) + '…' : text;
}

export const useAIChat = (): AIChatContextValue => {
    const context = useContext(AIChatContext);
    if (!context) {
        throw new Error('useAIChat must be used within an AIChatProvider');
    }
    return context;
};

let messageCounter = 0;

interface AIChatProviderProps {
    children: React.ReactNode;
}

export const AIChatProvider: React.FC<AIChatProviderProps> = ({ children }) => {
    const { currentContext, selectedNamespaces } = useK8s();
    const { activeView, bottomTabs, activeTabId } = useUI();
    const { getConfig } = useConfig();

    const [isOpen, setIsOpen] = useState<boolean>(false);
    const [messages, setMessages] = useState<ChatMessage[]>([]);
    const [isStreaming, setIsStreaming] = useState<boolean>(false);
    const [providerAvailable, setProviderAvailable] = useState<boolean | null>(null); // null = checking, true/false
    const [providerStatus, setProviderStatus] = useState<string>('');
    const [providerName, setProviderName] = useState<string>('');
    const [sessionId, setSessionId] = useState<string | null>(null);
    const [conversationId, setConversationId] = useState<string | null>(null);
    const [conversationHistory, setConversationHistory] = useState<ConversationHistoryItem[]>(() => loadConversationHistory());

    const streamingMessageRef = useRef<string | null>(null);
    const lastChunkTimeRef = useRef<number | null>(null);
    const streamStartTimeRef = useRef<number | null>(null);
    const autoExecutedNavRef = useRef<Set<string>>(new Set());
    const generationRef = useRef<number>(0);
    const currentUsageRef = useRef<TokenUsage | null>(null); // track latest token usage
    const sessionIdRef = useRef<string | null>(sessionId);
    sessionIdRef.current = sessionId;
    const THINKING_THRESHOLD = 500; // ms — pause longer than this inserts a thought bubble

    // Check provider on mount
    useEffect(() => {
        CheckAIProvider().then((result: AIProviderCheckResult) => {
            setProviderAvailable(result.available);
            setProviderStatus(result.status);
            setProviderName(result.provider || '');
        }).catch(() => {
            setProviderAvailable(false);
            setProviderStatus('Failed to check AI provider');
        });
    }, []);

    // Start session when panel opens
    useEffect(() => {
        if (isOpen && !sessionId && providerAvailable) {
            // Pass clientId for server mode session cleanup on disconnect
            const clientId = getClientId() || '';
            StartAISession(clientId).then((id: string) => {
                if (id) setSessionId(id);
            }).catch(err => console.error('Failed to start AI session:', err));
        }
    }, [isOpen, sessionId, providerAvailable]);

    // Listen for AI response events — registered once, uses refs to avoid stale closures
    useEffect(() => {
        const handler = (event: AIResponseEvent) => {
            if (!event || event.sessionId !== sessionIdRef.current) return;
            // Discard events from a previous (stale) generation
            if (event.generation && event.generation < generationRef.current) return;

            if (event.error) {
                setMessages(prev => {
                    // If streaming, mark the streaming message as error
                    if (streamingMessageRef.current) {
                        const msgId = streamingMessageRef.current;
                        streamingMessageRef.current = null;
                        return prev.map(m =>
                            m.id === msgId
                                ? { ...m, content: m.content + '\n\n[Error: ' + event.error + ']', streaming: false, isError: true }
                                : m
                        );
                    }
                    // Otherwise add a new error message
                    return [...prev, {
                        id: `msg-${++messageCounter}`,
                        role: 'assistant',
                        content: 'Error: ' + event.error,
                        streaming: false,
                        isError: true
                    }];
                });
                if (event.done) {
                    setIsStreaming(false);
                    streamingMessageRef.current = null;
                }
                return;
            }

            if (event.chunk) {
                const now = Date.now();
                const lastTime = lastChunkTimeRef.current;
                lastChunkTimeRef.current = now;

                // Track usage for attaching to final message
                if (event.usage) {
                    currentUsageRef.current = event.usage;
                }

                setMessages(prev => {
                    if (streamingMessageRef.current) {
                        // Detect thinking pause — split into thought bubble + new message
                        if (lastTime && (now - lastTime) > THINKING_THRESHOLD) {
                            const pauseSec = Math.round((now - lastTime) / 1000);
                            const oldMsgId = streamingMessageRef.current;
                            const thoughtId = `msg-${++messageCounter}`;
                            const newMsgId = `msg-${++messageCounter}`;
                            streamingMessageRef.current = newMsgId;
                            return [
                                ...prev.map(m => m.id === oldMsgId ? { ...m, streaming: false } : m),
                                { id: thoughtId, role: 'thought', content: `Thought for ${pauseSec}s`, streaming: false, isError: false },
                                { id: newMsgId, role: 'assistant', content: event.chunk, streaming: true, isError: false },
                            ];
                        }
                        // Append to existing streaming message
                        const msgId = streamingMessageRef.current;
                        return prev.map(m =>
                            m.id === msgId
                                ? { ...m, content: m.content + event.chunk }
                                : m
                        );
                    }
                    // Create new assistant message
                    const newId = `msg-${++messageCounter}`;
                    streamingMessageRef.current = newId;
                    const newMsg = { id: newId, role: 'assistant', content: event.chunk, streaming: true, isError: false };

                    // Record initial thinking time if significant
                    if (streamStartTimeRef.current && (now - streamStartTimeRef.current) > THINKING_THRESHOLD) {
                        const pauseSec = Math.round((now - streamStartTimeRef.current) / 1000);
                        const thoughtId = `msg-${++messageCounter}`;
                        streamStartTimeRef.current = null;
                        return [...prev,
                            { id: thoughtId, role: 'thought', content: `Thought for ${pauseSec}s`, streaming: false, isError: false },
                            newMsg,
                        ];
                    }
                    streamStartTimeRef.current = null;
                    return [...prev, newMsg];
                });
            }

            if (event.done) {
                // Update final usage
                if (event.usage) {
                    currentUsageRef.current = event.usage;
                }
                // Finalize streaming message and attach final usage
                if (streamingMessageRef.current) {
                    const msgId = streamingMessageRef.current;
                    const finalUsage = event.usage || currentUsageRef.current;
                    setMessages(prev => prev.map(m =>
                        m.id === msgId ? { ...m, streaming: false, usage: finalUsage } : m
                    ));
                }
                streamingMessageRef.current = null;
                lastChunkTimeRef.current = null;
                setIsStreaming(false);
            }
        };

        EventsOn('ai:response', handler);
        return () => EventsOff('ai:response');
    }, []); // eslint-disable-line react-hooks/exhaustive-deps — uses sessionIdRef to avoid re-registering

    // Clean up session on unmount
    useEffect(() => {
        return () => {
            if (sessionId) {
                CloseAISession(sessionId).catch(() => {});
            }
        };
    }, [sessionId]);

    // Auto-save current conversation to history when messages change
    useEffect(() => {
        // Only save if there are user messages (not just empty or only thought bubbles)
        const hasUserMessages = messages.some(m => m.role === 'user');
        if (!hasUserMessages) return;

        // Don't save while streaming
        if (isStreaming) return;

        setConversationHistory(prev => {
            const id = conversationId || `conv-${Date.now()}`;
            if (!conversationId) {
                setConversationId(id);
            }

            const updatedConv: ConversationHistoryItem = {
                id,
                title: generateTitle(messages),
                messages: messages.filter(m => m.role !== 'thought'), // Don't persist thought bubbles
                updatedAt: Date.now()
            };

            // Update existing or add new
            const existingIdx = prev.findIndex(c => c.id === id);
            let updated: ConversationHistoryItem[];
            if (existingIdx >= 0) {
                updated = [...prev];
                updated[existingIdx] = updatedConv;
            } else {
                updated = [updatedConv, ...prev];
            }

            // Keep only last MAX_CONVERSATIONS
            if (updated.length > MAX_CONVERSATIONS) {
                updated = updated.slice(0, MAX_CONVERSATIONS);
            }

            saveConversationHistory(updated);
            return updated;
        });
    }, [messages, isStreaming, conversationId]);

    // Build system prompt with K8s context info
    const buildSystemPrompt = useCallback(() => {
        const parts = [
            'You are a Kubernetes assistant integrated into Kubikles, a desktop Kubernetes client.',
            'Provide concise, practical answers about Kubernetes resources, troubleshooting, and best practices.',
            'The user may reference resources they are currently viewing. Use the context provided with each message to understand what they are looking at.',
            'You have tools to query this cluster directly — use them immediately to answer questions with real data instead of suggesting kubectl commands. All provided tools are pre-approved by the user; call them directly without asking first. Do not say "Would you like me to fetch…" or "Shall I check…" — just call the tool and present the results.',
            'Each message includes a "Relevant tools:" hint listing the best tools for the user\'s current view — prefer those tools when answering.',
            'If a tool is not available or fails with a permission error, mention its full MCP name in backticks (e.g. `mcp__kubikles__get_cluster_metrics`) so the user can enable it via the inline toggle. Do not ask for permission in prose — just reference the tool name in backticks and the UI will show an enable button.',
            'IMPORTANT: Never modify, delete, or update cluster resources without explicitly asking the user first and receiving confirmation. Read-only operations (get, list, describe) are always safe and pre-approved. For any mutating action (edit, delete, scale, patch, apply, restart), present the options and wait for the user to choose.',
            'You can create navigation links that are automatically executed when your response completes. Use markdown links with the nav:// scheme:' +
                '\n- Navigate to a list view: [Show pods](nav://view/pods)' +
                '\n- Search in a view: [Find nginx pods](nav://view/pods?search=nginx)' +
                '\n- Open YAML editor: [Edit YAML](nav://yaml/deployment/my-app/default)' +
                '\n- Open pod logs: [View logs](nav://logs/my-pod/default)' +
                '\n- Open resource details: [View details](nav://details/deployment/my-app/default)' +
                '\nValid view names: pods, deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs, configmaps, secrets, services, ingresses, nodes, namespaces, events, pvcs, pvs, storageclasses, hpas, pdbs, networkpolicies, serviceaccounts, roles, clusterroles, rolebindings, clusterrolebindings, helmreleases.' +
                '\nFor yaml/logs/details, use lowercase resource type (pod, deployment, configmap, etc.). Namespace is optional for cluster-scoped resources.' +
                '\nThese links are auto-executed so only include them for actions the user explicitly requested. The links also render as clickable buttons for re-use.' +
                '\n\nYou can also discover and navigate Custom Resource Definitions (CRDs) like Traefik IngressRoutes, cert-manager Certificates, etc.:' +
                '\n- Use the list_crds tool to discover what CRDs exist in the cluster' +
                '\n- Use list_custom_resources to list instances of a CRD (requires group, version, resource from list_crds)' +
                '\n- Use get_custom_resource_yaml to inspect a specific custom resource instance' +
                '\n- Navigate to a CRD list view: [Show IngressRoutes](nav://view/cr:traefik.io:v1alpha1:ingressroutes:IngressRoute:true)' +
                '\n  Format: nav://view/cr:GROUP:VERSION:PLURAL:KIND:NAMESPACED (NAMESPACED is true/false based on CRD scope)' +
                '\n- Search within a CRD view: [Find route](nav://view/cr:traefik.io:v1alpha1:ingressroutes:IngressRoute:true?search=my-route)' +
                '\n- Open CRD YAML: [Edit Kafka YAML](nav://yaml/cr:kafka.strimzi.io:v1beta2:kafkas:Kafka/my-kafka/my-namespace)' +
                '\n- Open CRD details: [View Kafka](nav://details/cr:kafka.strimzi.io:v1beta2:kafkas:Kafka/my-kafka/my-namespace)' +
                '\n  Format for CRD yaml/details: nav://yaml/cr:GROUP:VERSION:RESOURCE:KIND/NAME/NAMESPACE' +
                '\n  Use the CRD metadata from list_crds (group, version, plural name, kind) to construct these links.',
        ];
        // Add safety clause when dangerous CLI tools are enabled
        const dangerousTools = ['Bash', 'Write', 'Edit'];
        const allowedTools = (getConfig('ai.allowedTools') as string[] | undefined) || [];
        const hasDangerousTools = dangerousTools.some(t => allowedTools.includes(t));
        if (hasDangerousTools) {
            const enabled = dangerousTools.filter(t => allowedTools.includes(t)).join(', ');
            parts.push(
                `SAFETY: The following powerful tools are enabled: ${enabled}. ` +
                'Before executing any file modification or shell command, describe exactly what you intend to do and wait for the user to confirm. ' +
                'Never run destructive commands (rm, kubectl delete, DROP, etc.) without explicit user approval.'
            );
        }
        if (currentContext) {
            parts.push(`The user is connected to K8s context: "${currentContext}".`);
        }
        if (selectedNamespaces?.length > 0) {
            parts.push(`Selected namespaces: ${selectedNamespaces.join(', ')}.`);
        }
        return parts.join(' ');
    }, [currentContext, selectedNamespaces, getConfig]);

    // Build context string describing what the user is currently looking at
    const buildMessageContext = useCallback(() => {
        const lines = [];
        if (activeView) {
            lines.push(`Resource list view: ${activeView}`);
        }
        // Describe the active bottom panel tab using structured resourceMeta when available
        const activeTab = bottomTabs.find(t => t.id === activeTabId);
        let tabAction = null;
        if (activeTab) {
            const meta = activeTab.resourceMeta;
            const parsed = parseTabId(activeTab.id);
            tabAction = parsed?.action || null;
            if (meta) {
                // Build a rich context line from structured resource metadata
                const actionMap = { details: 'viewing details of', yaml: 'editing YAML of', logs: 'viewing logs of', deps: 'viewing dependencies of', terminal: 'shell into', files: 'browsing files of' };
                const action = parsed?.action || activeTab.actionLabel?.toLowerCase() || 'viewing';
                const actionDesc = actionMap[action] || `viewing ${action} of`;
                const nsPart = meta.namespace ? ` in namespace "${meta.namespace}"` : ' (cluster-scoped)';
                lines.push(`Open tab: ${actionDesc} ${meta.kind} "${meta.name}"${nsPart}`);
            } else if (parsed) {
                const actionMap = { details: 'viewing details of', yaml: 'editing YAML of', logs: 'viewing logs of', deps: 'viewing dependencies of' };
                const actionDesc = actionMap[parsed.action] || `viewing ${parsed.action} of`;
                lines.push(`Open tab: ${actionDesc} ${parsed.resourceType} "${activeTab.title}"`);
            } else {
                lines.push(`Open tab: "${activeTab.title}"`);
            }
        }

        // Compute relevant tools from view + tab
        const toolSet = new Set();
        const viewKey = activeView?.startsWith('cr:') ? 'cr:' : activeView;
        const viewTools = viewKey === 'cr:'
            ? ['list_custom_resources', 'get_custom_resource_yaml']
            : VIEW_TOOL_MAP[viewKey];
        if (viewTools) viewTools.forEach(t => toolSet.add(t));
        if (tabAction && TAB_ACTION_TOOL_MAP[tabAction]) {
            TAB_ACTION_TOOL_MAP[tabAction].forEach(t => toolSet.add(t));
        }
        // Kind-specific metrics tools for detail tabs
        if (activeTab?.resourceMeta?.kind) {
            const kind = activeTab.resourceMeta.kind.toLowerCase();
            if (['pod'].includes(kind)) toolSet.add('get_pod_metrics');
            if (['node'].includes(kind)) toolSet.add('get_cluster_metrics');
            if (['deployment', 'statefulset', 'daemonset'].includes(kind)) toolSet.add('get_pod_metrics');
            if (['namespace'].includes(kind)) toolSet.add('get_namespace_summary');
        }
        if (toolSet.size > 0) {
            lines.push(`Relevant tools for this view: ${[...toolSet].join(', ')}`);
        }

        return lines.length > 0 ? lines.join('. ') : '';
    }, [activeView, bottomTabs, activeTabId]);

    const togglePanel = useCallback(() => {
        setIsOpen(prev => !prev);
    }, []);

    const sendMessage = useCallback((text: string) => {
        if (!text.trim() || !sessionId || isStreaming) return;

        // Add user message
        const userMsg: ChatMessage = {
            id: `msg-${++messageCounter}`,
            role: 'user',
            content: text.trim(),
            streaming: false,
            isError: false
        };
        setMessages(prev => [...prev, userMsg]);
        generationRef.current++;
        currentUsageRef.current = null;

        const model = getConfig('ai.model') || 'sonnet';
        const systemPrompt = buildSystemPrompt();

        // Prepend current UI context to the message sent to the provider
        // so the AI knows what the user is looking at right now
        const ctx = buildMessageContext();
        const fullMessage = ctx
            ? `[Context: ${ctx}]\n\n${text.trim()}`
            : text.trim();

        // Build fully-qualified tool names from config
        // CLI built-in tools (Bash, WebSearch, etc.) pass through as-is
        // Short names without __ get the mcp__kubikles__ prefix
        const cliBuiltinTools = new Set(['Bash', 'WebSearch', 'Read', 'Write', 'Edit', 'Glob', 'Grep']);
        const allowedTools = ((getConfig('ai.allowedTools') as string[] | undefined) || [])
            .map(t => t.includes('__') || cliBuiltinTools.has(t) ? t : `mcp__kubikles__${t}`);

        const timeoutSeconds = ((getConfig('ai.requestTimeout') as number | undefined) || 10) * 60;

        // Only set isStreaming if request was successfully initiated
        SendAIMessage(sessionId, fullMessage, systemPrompt, model, allowedTools, timeoutSeconds)
            .then((success: boolean) => {
                if (success) {
                    setIsStreaming(true);
                    streamStartTimeRef.current = Date.now();
                }
                // If not successful, error event will be emitted by backend
            })
            .catch(err => {
                console.error('Failed to send AI message:', err);
            });
    }, [sessionId, isStreaming, buildSystemPrompt, buildMessageContext, getConfig]);

    const cancelRequest = useCallback(() => {
        if (sessionId) {
            CancelAIRequest(sessionId).catch(() => {});
            setIsStreaming(false);
            if (streamingMessageRef.current) {
                const msgId = streamingMessageRef.current;
                setMessages(prev => prev.map(m =>
                    m.id === msgId ? { ...m, streaming: false, content: m.content + '\n\n[Cancelled]' } : m
                ));
                streamingMessageRef.current = null;
            }
        }
    }, [sessionId]);

    // Reset UI state for a new/loaded conversation
    const resetChatState = useCallback(() => {
        setMessages([]);
        setIsStreaming(false);
        streamingMessageRef.current = null;
        lastChunkTimeRef.current = null;
        streamStartTimeRef.current = null;
        generationRef.current = 0;
        autoExecutedNavRef.current.clear();
        setConversationId(null);
    }, []);

    // Start a new chat (current conversation is auto-saved by the effect)
    const startNewChat = useCallback(() => {
        if (sessionId) {
            // Clear backend session and get new ID
            ClearAISession(sessionId).then((newId: string) => {
                resetChatState();
                if (newId) setSessionId(newId);
            }).catch(() => {
                resetChatState();
            });
        } else {
            resetChatState();
        }
    }, [sessionId, resetChatState]);

    // Load a conversation from history
    const loadConversation = useCallback((convId: string) => {
        const conv = conversationHistory.find(c => c.id === convId);
        if (!conv) return;

        // Reset message counter to avoid ID collisions
        const maxMsgId = conv.messages.reduce((max, m) => {
            const num = parseInt(m.id?.replace('msg-', '') || '0', 10);
            return num > max ? num : max;
        }, 0);
        messageCounter = Math.max(messageCounter, maxMsgId);

        // Helper to load the conversation state
        const loadState = () => {
            setMessages(conv.messages);
            setConversationId(convId);
            setIsStreaming(false);
            streamingMessageRef.current = null;
            lastChunkTimeRef.current = null;
            streamStartTimeRef.current = null;
            generationRef.current = 0;
            autoExecutedNavRef.current.clear();
        };

        if (sessionId) {
            // ClearAISession cancels any pending request and creates fresh session
            ClearAISession(sessionId).then((newId: string) => {
                loadState();
                if (newId) setSessionId(newId);
            }).catch(() => {
                loadState();
            });
        } else {
            loadState();
        }
    }, [sessionId, conversationHistory]);

    // Delete a conversation from history
    const deleteConversation = useCallback((convId: string) => {
        setConversationHistory(prev => {
            const updated = prev.filter(c => c.id !== convId);
            saveConversationHistory(updated);
            return updated;
        });
        // If deleting current conversation, start fresh
        if (convId === conversationId) {
            resetChatState();
        }
    }, [conversationId, resetChatState]);

    const value = useMemo(() => ({
        isOpen,
        togglePanel,
        messages,
        sendMessage,
        isStreaming,
        cancelRequest,
        startNewChat,
        loadConversation,
        deleteConversation,
        conversationHistory,
        conversationId,
        providerAvailable,
        providerStatus,
        providerName,
        autoExecutedNavRef
    }), [isOpen, togglePanel, messages, sendMessage, isStreaming, cancelRequest, startNewChat, loadConversation, deleteConversation, conversationHistory, conversationId, providerAvailable, providerStatus, providerName]);

    return (
        <AIChatContext.Provider value={value}>
            {children}
        </AIChatContext.Provider>
    );
};
