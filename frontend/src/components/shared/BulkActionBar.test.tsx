import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import BulkActionBar from './BulkActionBar';
import { bulkResourceLabel } from './ResourceList';

describe('BulkActionBar selection text', () => {
    it('reads the plural resource name for custom resources, not the list id', () => {
        const label = bulkResourceLabel('cr-monitoring.coreos.com-prometheusrules', 'PrometheusRule');
        const html = renderToStaticMarkup(<BulkActionBar selectedCount={37} resourceType="prometheusrules" onClearSelection={() => {}} />);
        expect(html).toContain('37 prometheusrules selected');
        expect(html).not.toContain('cr-monitoring');
        expect(label).not.toContain('cr-');
    });

    it('singularises one selected row', () => {
        const html = renderToStaticMarkup(<BulkActionBar selectedCount={1} resourceType="prometheusrules" onClearSelection={() => {}} />);
        expect(html).toContain('1 prometheusrule selected');
    });
});
