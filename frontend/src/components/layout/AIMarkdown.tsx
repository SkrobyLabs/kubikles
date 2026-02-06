import React, { useCallback } from 'react';
import { ArrowTopRightOnSquareIcon, CubeIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useConfig } from '~/context';
import { LazyYamlEditor } from '../lazy';
import LogViewer from '../shared/log-viewer';
import { getResourceIcon } from '~/utils/resourceIcons';
import { configSchema } from '~/config/configSchema';
import { GetCustomResourceYaml, UpdateCustomResourceYaml } from 'wailsjs/go/main/App';
import { parseCrKind, kindToViewName } from './navUtils';

// Execute a nav:// link — shared by NavLink click handler and auto-execution
export function executeNavLink(href, { setActiveView, navigateWithSearch, openTab, closeTab, currentContext }) {
    const raw = href.slice('nav://'.length);
    const [pathPart, queryPart] = raw.split('?');
    const parts = pathPart.split('/').filter(Boolean);
    const params = new URLSearchParams(queryPart || '');

    switch (parts[0]) {
        case 'view': {
            if (!parts[1]) break;
            const search = params.get('search');
            if (search) {
                navigateWithSearch(parts[1], search);
            } else {
                setActiveView(parts[1]);
            }
            break;
        }
        case 'yaml': {
            if (!parts[1] || !parts[2]) break;
            const kind = parts[1];
            const name = parts[2];
            const namespace = parts[3];
            const cr = parseCrKind(kind);
            const displayKind = cr ? cr.kind : kind;
            const tabId = cr
                ? `cr-yaml-${cr.group}-${cr.resource}-${namespace || ''}-${name}`
                : `yaml-${kind}-${name}`;
            openTab({
                id: tabId,
                title: cr ? `${name} (${cr.kind})` : name,
                icon: getResourceIcon(displayKind) || CubeIcon,
                actionLabel: 'Edit',
                content: (
                    <LazyYamlEditor
                        resourceType={cr ? 'customresource' : kind}
                        namespace={namespace}
                        resourceName={name}
                        onClose={() => closeTab(tabId)}
                        tabContext={currentContext}
                        {...(cr && {
                            getYamlFn: () => GetCustomResourceYaml(cr.group, cr.version, cr.resource, namespace, name),
                            updateYamlFn: (content) => UpdateCustomResourceYaml(cr.group, cr.version, cr.resource, namespace, name, content),
                        })}
                    />
                ),
                resourceMeta: { kind: displayKind, name, namespace },
            });
            break;
        }
        case 'logs': {
            if (!parts[1]) break;
            const podName = parts[1];
            const namespace = parts[2];
            const tabId = `logs-pod-${podName}`;
            openTab({
                id: tabId,
                title: podName,
                icon: CubeIcon,
                actionLabel: 'Logs',
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={podName}
                        containers={[]}
                        siblingPods={[]}
                        podContainerMap={{}}
                        ownerName=""
                        podCreationTime=""
                        tabContext={currentContext}
                    />
                ),
                resourceMeta: { kind: 'Pod', name: podName, namespace },
            });
            break;
        }
        case 'details': {
            if (!parts[1] || !parts[2]) break;
            const kind = parts[1];
            const name = parts[2];
            const namespace = parts[3];
            const cr = parseCrKind(kind);
            if (cr) {
                const namespaced = namespace ? 'true' : 'false';
                const viewId = `cr:${cr.group}:${cr.version}:${cr.resource}:${cr.kind}:${namespaced}`;
                navigateWithSearch(viewId, name, true);
            } else {
                const viewName = kindToViewName[kind.toLowerCase()] || kind.toLowerCase() + 's';
                navigateWithSearch(viewName, name, true);
            }
            break;
        }
    }
}

// Navigation link component — renders nav:// links as clickable buttons
function NavLink({ href, children }) {
    const { setActiveView, navigateWithSearch, openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleClick = useCallback(() => {
        executeNavLink(href, { setActiveView, navigateWithSearch, openTab, closeTab, currentContext });
    }, [href, setActiveView, navigateWithSearch, openTab, closeTab, currentContext]);

    return (
        <button
            onClick={handleClick}
            className="inline-flex items-center gap-1 px-1.5 py-0.5 my-0.5 rounded bg-primary/15 text-primary hover:bg-primary/25 text-xs font-medium transition-colors cursor-pointer"
        >
            {children}
            <ArrowTopRightOnSquareIcon className="h-3 w-3 shrink-0" />
        </button>
    );
}

// Inline toggle button for allowing/disallowing a tool mentioned in AI chat
function ToolMention({ toolName }) {
    const { getConfig, setConfig } = useConfig();
    // Normalize to short name for storage (kubikles tools use short names)
    const shortName = toolName.startsWith('mcp__kubikles__')
        ? toolName.slice('mcp__kubikles__'.length)
        : knownToolNames.has(toolName) ? toolName
        : toolName; // external tools keep full name
    const displayName = knownToolNames.has(shortName) && !toolName.includes('__')
        ? shortName : toolName;
    const allowedTools = getConfig('ai.allowedTools') || [];
    const isAllowed = allowedTools.includes(shortName);

    const toggle = (e) => {
        e.stopPropagation();
        const updated = isAllowed
            ? allowedTools.filter(t => t !== shortName)
            : [...allowedTools, shortName];
        setConfig('ai.allowedTools', updated);
    };

    return (
        <span className="inline-flex items-center gap-1">
            <code className="px-1 py-0.5 rounded bg-black/30 font-mono text-[11px]">{displayName}</code>
            <button onClick={toggle} className={isAllowed
                ? "text-[10px] px-1.5 py-0.5 rounded bg-green-500/20 text-green-400"
                : "text-[10px] px-1.5 py-0.5 rounded bg-primary/20 text-primary hover:bg-primary/30"}>
                {isAllowed ? '\u2713 Allowed' : 'Allow'}
            </button>
        </span>
    );
}

// Render inline markdown: **bold**, `code`, [nav links](nav://...)
const inlinePatterns = [
    { regex: /\*\*(.+?)\*\*/, type: 'bold' },
    { regex: /`([^`]+)`/, type: 'code' },
    { regex: /\[([^\]]+)\]\(([^)]+)\)/, type: 'link' },
];

const mcpToolPattern = /^mcp__\w+__\w+$/;
// Known kubikles short tool names for broader detection
const knownToolNames = new Set(
    (configSchema.ai?.allowedTools?.options || []).map(o => o.value)
);

function renderInline(text) {
    const parts = [];
    let remaining = text;
    let key = 0;

    while (remaining.length > 0) {
        // Find the earliest matching pattern
        let earliest = null;
        for (const p of inlinePatterns) {
            const m = remaining.match(p.regex);
            if (m && (earliest === null || m.index < earliest.match.index)) {
                earliest = { match: m, type: p.type };
            }
        }

        if (!earliest) {
            parts.push(remaining);
            break;
        }

        const { match, type } = earliest;
        if (match.index > 0) parts.push(remaining.slice(0, match.index));

        switch (type) {
            case 'bold':
                parts.push(<strong key={key++} className="font-semibold">{renderInline(match[1])}</strong>);
                break;
            case 'code':
                if (mcpToolPattern.test(match[1]) || knownToolNames.has(match[1])) {
                    parts.push(<ToolMention key={key++} toolName={match[1]} />);
                } else {
                    parts.push(<code key={key++} className="px-1 py-0.5 rounded bg-black/30 font-mono text-[11px]">{match[1]}</code>);
                }
                break;
            case 'link':
                if (match[2].startsWith('nav://')) {
                    parts.push(<NavLink key={key++} href={match[2]}>{match[1]}</NavLink>);
                } else {
                    parts.push(match[1]);
                }
                break;
        }

        remaining = remaining.slice(match.index + match[0].length);
    }

    return parts.length === 1 && typeof parts[0] === 'string' ? parts[0] : parts;
}

// Lightweight markdown renderer for AI responses
export default function SimpleMarkdown({ text }) {
    if (!text) return null;

    const lines = text.split('\n');
    const elements = [];
    let i = 0;

    while (i < lines.length) {
        const line = lines[i];

        // Fenced code block
        if (line.trimStart().startsWith('```')) {
            const lang = line.trimStart().slice(3).trim();
            const codeLines = [];
            i++;
            while (i < lines.length && !lines[i].trimStart().startsWith('```')) {
                codeLines.push(lines[i]);
                i++;
            }
            i++; // skip closing ```
            elements.push(
                <div key={elements.length} className="my-1.5 rounded bg-black/30 overflow-x-auto">
                    {lang && <div className="text-[10px] text-gray-500 px-2 pt-1">{lang}</div>}
                    <pre className="px-2 py-1.5 text-xs font-mono whitespace-pre">{codeLines.join('\n')}</pre>
                </div>
            );
            continue;
        }

        // Empty line → spacer
        if (line.trim() === '') {
            elements.push(<div key={elements.length} className="h-1.5" />);
            i++;
            continue;
        }

        // Heading
        const headingMatch = line.match(/^(#{1,3})\s+(.+)/);
        if (headingMatch) {
            const level = headingMatch[1].length;
            const cls = level === 1 ? 'text-sm font-bold mt-2 mb-1' : level === 2 ? 'text-xs font-bold mt-1.5 mb-0.5' : 'text-xs font-semibold mt-1 mb-0.5';
            elements.push(<div key={elements.length} className={cls}>{renderInline(headingMatch[2])}</div>);
            i++;
            continue;
        }

        // Bullet list — collect consecutive bullet lines
        if (/^\s*[-*]\s/.test(line)) {
            const items = [];
            while (i < lines.length && /^\s*[-*]\s/.test(lines[i])) {
                items.push(lines[i].replace(/^\s*[-*]\s+/, ''));
                i++;
            }
            elements.push(
                <ul key={elements.length} className="list-disc list-inside space-y-0.5 my-0.5">
                    {items.map((item, j) => <li key={j} className="text-xs">{renderInline(item)}</li>)}
                </ul>
            );
            continue;
        }

        // Numbered list
        if (/^\s*\d+[.)]\s/.test(line)) {
            const items = [];
            while (i < lines.length && /^\s*\d+[.)]\s/.test(lines[i])) {
                items.push(lines[i].replace(/^\s*\d+[.)]\s+/, ''));
                i++;
            }
            elements.push(
                <ol key={elements.length} className="list-decimal list-inside space-y-0.5 my-0.5">
                    {items.map((item, j) => <li key={j} className="text-xs">{renderInline(item)}</li>)}
                </ol>
            );
            continue;
        }

        // Regular paragraph line
        elements.push(<p key={elements.length} className="text-xs leading-relaxed">{renderInline(line)}</p>);
        i++;
    }

    return <>{elements}</>;
}
