/**
 * Search Module
 *
 * Advanced search functionality for Kubernetes resources.
 *
 * @example
 * import { createFilter } from '~/utils/search';
 *
 * const filterFn = createFilter('pods', 'name:"nginx" status:Running');
 * const filtered = pods.filter(filterFn);
 */

export { parseQuery } from './queryParser';
export { createFilter, filterData } from './filterEngine';
export {
    getFieldByName,
    getFieldsForResource,
    getAvailableFieldNames,
    getFieldsMetadata
} from './fieldMappings';
