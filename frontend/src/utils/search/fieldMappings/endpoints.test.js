import { describe, it, expect } from 'vitest';
import { endpointsFields } from './endpoints';

describe('endpointsFields', () => {
    describe('subsets', () => {
        it('counts subsets correctly', () => {
            expect(endpointsFields.subsets.extractor({ subsets: [{}, {}, {}] })).toBe('3');
            expect(endpointsFields.subsets.extractor({})).toBe('0');
        });
    });

    describe('addresses', () => {
        it('extracts IP addresses from all subsets', () => {
            const ep = {
                subsets: [
                    { addresses: [{ ip: '10.0.0.1' }, { ip: '10.0.0.2' }] },
                    { addresses: [{ ip: '10.0.0.3' }] }
                ]
            };
            const result = endpointsFields.addresses.extractor(ep);
            expect(result).toContain('10.0.0.1');
            expect(result).toContain('10.0.0.2');
            expect(result).toContain('10.0.0.3');
        });

        it('handles empty subsets', () => {
            expect(endpointsFields.addresses.extractor({ subsets: [] })).toBe('');
        });
    });

    describe('ready', () => {
        it('counts ready addresses', () => {
            const ep = {
                subsets: [
                    { addresses: [{ ip: '10.0.0.1' }, { ip: '10.0.0.2' }] },
                    { addresses: [{ ip: '10.0.0.3' }] }
                ]
            };
            expect(endpointsFields.ready.extractor(ep)).toBe('3');
        });
    });

    describe('notready', () => {
        it('counts not ready addresses', () => {
            const ep = {
                subsets: [
                    { notReadyAddresses: [{ ip: '10.0.0.1' }] },
                    { notReadyAddresses: [{ ip: '10.0.0.2' }, { ip: '10.0.0.3' }] }
                ]
            };
            expect(endpointsFields.notready.extractor(ep)).toBe('3');
        });
    });

    describe('ports', () => {
        it('extracts ports from subsets', () => {
            const ep = {
                subsets: [
                    { ports: [{ port: 80, protocol: 'TCP' }, { port: 443, protocol: 'TCP' }] },
                    { ports: [{ port: 8080 }] }
                ]
            };
            const result = endpointsFields.ports.extractor(ep);
            expect(result).toContain('80/TCP');
            expect(result).toContain('443/TCP');
            expect(result).toContain('8080/TCP');
        });
    });

    describe('targetref', () => {
        it('extracts target references', () => {
            const ep = {
                subsets: [{
                    addresses: [
                        { ip: '10.0.0.1', targetRef: { kind: 'Pod', name: 'pod-1' } },
                        { ip: '10.0.0.2', targetRef: { kind: 'Pod', name: 'pod-2' } }
                    ]
                }]
            };
            const result = endpointsFields.targetref.extractor(ep);
            expect(result).toContain('Pod/pod-1');
            expect(result).toContain('Pod/pod-2');
        });
    });
});
