const fs = require('fs');
const path = require('path');

function fixFile(filepath) {
    let content = fs.readFileSync(filepath, 'utf8');
    const original = content;

    // 1. Fix function component props: ({ isVisible })
    content = content.replace(
        /export default function (\w+)\(\{ isVisible \}\)/g,
        'export default function $1({ isVisible }: { isVisible: boolean })'
    );

    // 2. Fix (item) => in column render/getValue
    content = content.replace(/render: \(item\) =>/g, 'render: (item: any) =>');
    content = content.replace(/getValue: \(item\) =>/g, 'getValue: (item: any) =>');

    // 3. Fix onOpenChange={(isOpen, buttonElement) =>
    content = content.replace(
        /onOpenChange=\{\(isOpen, buttonElement\) =>/g,
        'onOpenChange={(isOpen: any, buttonElement: any) =>'
    );

    // 4. Fix catch (err) {
    content = content.replace(/\} catch \(err\) \{/g, '} catch (err: any) {');

    // 5. Fix useState(null)
    content = content.replace(/useState\(null\)/g, 'useState<any>(null)');

    // 6. Fix useState([])
    content = content.replace(/useState\(\[\]\)/g, 'useState<any[]>([])');

    // 7. Fix onChange={(val) =>
    content = content.replace(/onChange=\{\(val\) =>/g, 'onChange={(val: any) =>');

    // 8. Fix getOptionValue/Label callbacks
    content = content.replace(/getOptionValue=\{\((\w+)\) =>/g, 'getOptionValue={($1: any) =>');
    content = content.replace(/getOptionLabel=\{\((\w+)\) =>/g, 'getOptionLabel={($1: any) =>');

    // 9. Fix .map/.find/.filter/.some/.forEach/.flatMap with single params (avoid already typed)
    const arrayMethods = ['map', 'find', 'filter', 'some', 'forEach', 'flatMap', 'every', 'sort', 'findIndex'];
    for (const method of arrayMethods) {
        // Single param without type: .method(x =>  but NOT .method((x: any) =>
        const regex = new RegExp(`\\.${method}\\((\\w+) =>`, 'g');
        content = content.replace(regex, `.${method}(($1: any) =>`);
    }
    // .reduce((acc, item) =>
    content = content.replace(/\.reduce\(\((\w+), (\w+)\) =>/g, '.reduce(($1: any, $2: any) =>');

    // 10. Fix .map((item, idx) => and similar two-param callbacks
    content = content.replace(/\.map\((\w+), (\w+)\) =>/g, '.map(($1: any, $2: number) =>');
    content = content.replace(/\.forEach\((\w+), (\w+)\) =>/g, '.forEach(($1: any, $2: number) =>');
    content = content.replace(/\.filter\((\w+), (\w+)\) =>/g, '.filter(($1: any, $2: number) =>');

    // 11. Fix useCallback parameters - common patterns
    content = content.replace(/useCallback\(async \((\w+)\) =>/g, 'useCallback(async ($1: any) =>');
    content = content.replace(/useCallback\(\((\w+)\) =>/g, 'useCallback(($1: any) =>');
    content = content.replace(/useCallback\(async \((\w+), (\w+)\) =>/g, 'useCallback(async ($1: any, $2: any) =>');
    content = content.replace(/useCallback\(\((\w+), (\w+)\) =>/g, 'useCallback(($1: any, $2: any) =>');
    content = content.replace(/useCallback\(async \((\w+), (\w+), (\w+)\) =>/g, 'useCallback(async ($1: any, $2: any, $3: any) =>');

    // 12. Fix .catch((err) =>
    content = content.replace(/\.catch\(\(err\) =>/g, '.catch((err: any) =>');

    // 13. Fix onDelete={(xxx) =>
    content = content.replace(/onDelete=\{\((\w+)\) =>/g, 'onDelete={($1: any) =>');

    // 14. Fix action={bulkActionModal.action} -> action={bulkActionModal.action || ''}
    content = content.replace(/action=\{bulkActionModal\.action\}/g, "action={bulkActionModal.action || ''}");

    // 15. Fix SearchSelect callbacks: onChange={(val) =>  (already covered by #7)
    // Fix onClear and similar callbacks
    content = content.replace(/onSelect=\{\((\w+)\) =>/g, 'onSelect={($1: any) =>');

    // Don't double-annotate
    content = content.replace(/: any: any/g, ': any');
    content = content.replace(/: number: any/g, ': number');
    content = content.replace(/: number: number/g, ': number');
    content = content.replace(/useState<any><any>/g, 'useState<any>');
    content = content.replace(/useState<any\[\]><any\[\]>/g, 'useState<any[]>');
    // Fix double-wrapped parens from array method regex: .map(((x: any)) =>
    content = content.replace(/\(\((\w+: any)\)\)/g, '($1)');

    if (content !== original) {
        fs.writeFileSync(filepath, content);
        return true;
    }
    return false;
}

function walkDir(dir, ext) {
    const files = [];
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const fullPath = path.join(dir, entry.name);
        if (entry.isDirectory()) {
            files.push(...walkDir(fullPath, ext));
        } else if (entry.name.endsWith(ext)) {
            files.push(fullPath);
        }
    }
    return files;
}

// Fix features dir
const featuresDir = path.join(__dirname, 'src', 'features');
const sharedDir = path.join(__dirname, 'src', 'components', 'shared');
const hooksDir = path.join(__dirname, 'src', 'hooks');
const utilsDir = path.join(__dirname, 'src', 'utils');
const contextDir = path.join(__dirname, 'src', 'context');
const layoutDir = path.join(__dirname, 'src', 'components', 'layout');

const dirs = [featuresDir, sharedDir, hooksDir, utilsDir, contextDir, layoutDir];
let changed = 0;
for (const dir of dirs) {
    if (!fs.existsSync(dir)) continue;
    const tsxFiles = walkDir(dir, '.tsx');
    const tsFiles = walkDir(dir, '.ts');
    const allFiles = [...tsxFiles, ...tsFiles];
    for (const f of allFiles) {
        if (fixFile(f)) {
            console.log('Fixed: ' + path.relative(__dirname, f));
            changed++;
        }
    }
}

// Fix App.tsx and main.tsx
const rootFiles = ['src/App.tsx', 'src/main.tsx'];
for (const rf of rootFiles) {
    const fp = path.join(__dirname, rf);
    if (fs.existsSync(fp) && fixFile(fp)) {
        console.log('Fixed: ' + rf);
        changed++;
    }
}

console.log('Total files modified: ' + changed);
