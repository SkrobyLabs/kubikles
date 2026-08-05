/** @vitest-environment jsdom */
import React from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ConfigSection from './ConfigSection';

vi.mock('~/context', () => ({ useTheme: () => ({ currentTheme: null, themes: [], switchTheme: vi.fn() }) }));
vi.mock('wailsjs/go/main/App', () => ({ GetCrashLogPath: vi.fn(), OpenCrashLogDir: vi.fn(), GetIssueRulesDir: vi.fn(), OpenIssueRulesDir: vi.fn() }));

const config = { accelerator: { enabledByDefault: false, defaultNamespace: '', development: { releaseVersion: '', descriptorURL: '', versionPolicy: 'exact' }, connectionOverrides: [{ contextName: 'prod', enabled: true }] } };

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

  it('shows a warning when Accelerator development overrides are active', () => {
    render(<ConfigSection section="accelerator" config={{ accelerator: { ...config.accelerator, development: { ...config.accelerator.development, versionPolicy: 'warn' } } }} onFieldChange={vi.fn()} searchResults={null} />);
    expect(screen.getByRole('alert').textContent).toContain('development overrides are active');
  });
});
