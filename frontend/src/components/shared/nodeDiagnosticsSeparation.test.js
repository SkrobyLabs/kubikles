import { describe, expect, it } from 'vitest';
import { readFileSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const componentDir = dirname(fileURLToPath(import.meta.url));

const readComponent = (name) => readFileSync(join(componentDir, name), 'utf8');

describe('node diagnostics request separation', () => {
    it('keeps diagnostic Wails calls out of the Metrics overview owner', () => {
        const metricsTab = readComponent('NodeMetricsTab.tsx');

        expect(metricsTab).toContain('GetNodeMetricsHistory');
        expect(metricsTab).not.toContain('GetNodeMemoryDiagnosticsHistory');
        expect(metricsTab).not.toContain('NodeMemoryDiagnosticView');
        expect(metricsTab).not.toContain('Memory diagnostics');
    });

    it('loads diagnostic history only from the Diagnostics tab owner', () => {
        const diagnosticsTab = readComponent('NodeMemoryDiagnosticsTab.tsx');

        expect(diagnosticsTab).toContain('GetNodeMemoryDiagnosticsHistory');
        expect(diagnosticsTab).toContain('GetNodeMemoryDiagnosticsHistoryRange');
        expect(diagnosticsTab).toContain('node-memory-diagnostics-${nodeName}');
        expect(diagnosticsTab).toContain('CancelMetricsRequest(requestId)');
    });

    it('exposes Diagnostics next to Basic and Metrics at the node level', () => {
        const nodeDetails = readComponent('NodeDetails.tsx');

        expect(nodeDetails).toContain("const TAB_BASIC = 'basic'");
        expect(nodeDetails).toContain("const TAB_METRICS = 'metrics'");
        expect(nodeDetails).toContain("const TAB_DIAGNOSTICS = 'diagnostics'");
        expect(nodeDetails).toContain("{ id: TAB_DIAGNOSTICS, label: 'Diagnostics' }");
        expect(nodeDetails).toContain('NodeMemoryDiagnosticsTab');
    });
});
