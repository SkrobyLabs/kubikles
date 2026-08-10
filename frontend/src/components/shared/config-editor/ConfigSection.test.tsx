/** @vitest-environment jsdom */
import React from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ConfigSection from './ConfigSection';

vi.mock('~/context', () => ({ useTheme: () => ({ currentTheme: null, themes: [], switchTheme: vi.fn() }) }));
vi.mock('wailsjs/go/main/App', () => ({ GetCrashLogPath: vi.fn(), OpenCrashLogDir: vi.fn(), GetIssueRulesDir: vi.fn(), OpenIssueRulesDir: vi.fn() }));

const config = { accelerator: { enabledByDefault: false, defaultNamespace: '', customImage: '', customChart: '', connectionOverrides: [{ contextName: 'prod', enabled: true }] } };

describe('ConfigSection', () => {
  it('does not render hidden Accelerator storage', () => {
    const { container } = render(<ConfigSection section="accelerator" config={config} onFieldChange={vi.fn()} searchResults={null} />);
    expect(screen.getByText('Deployment Namespace')).toBeTruthy();
    expect(screen.getByText('Enable Accelerator by default')).toBeTruthy();
    expect(screen.queryByText('connectionOverrides')).toBeNull();
    expect(container.querySelectorAll('input[type="text"]')).toHaveLength(3);
  });

  it('explains the Accelerator deployment contract without checkbox helper text', () => {
    render(<ConfigSection section="accelerator" config={config} onFieldChange={vi.fn()} searchResults={null} />);
    expect(screen.queryByText(/Checked deploys Accelerator/)).toBeNull();
    expect(screen.getByText(/in-cluster workload near the Kubernetes API server/)).toBeTruthy();
    expect(screen.getByText(/Secret reads only/)).toBeTruthy();
    expect(screen.getByText(/Direct remains authoritative/)).toBeTruthy();
    expect(screen.getByText(/Helm-resource permissions and cluster-wide read-only Secret RBAC/)).toBeTruthy();
    expect(screen.getByText(/Job and its supporting resources/)).toBeTruthy();
  });

  it('shows a warning when custom Accelerator artifacts are active', () => {
    render(<ConfigSection section="accelerator" config={{ accelerator: { ...config.accelerator, customImage: 'ghcr.io/example/accelerator:test' } }} onFieldChange={vi.fn()} searchResults={null} />);
    expect(screen.getByRole('alert').textContent).toContain('Custom Accelerator artifacts are active');
  });

  it('renders Accelerator artifact overrides at twice the default text width', () => {
    render(<ConfigSection section="accelerator" config={config} onFieldChange={vi.fn()} searchResults={null} />);
    expect(screen.getByPlaceholderText('ghcr.io/skrobylabs/kubikles-accelerator:v1.4.0-alpha.2').className).toContain('w-[32rem]');
    expect(screen.getByPlaceholderText('ghcr.io/skrobylabs/helm/kubikles-accelerator:1.4.0-alpha.2').className).toContain('w-[32rem]');
  });
});
