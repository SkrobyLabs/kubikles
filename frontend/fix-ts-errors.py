#!/usr/bin/env python3
"""Fix common TypeScript implicit 'any' errors in feature files."""
import re
import sys
import os

def fix_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    original = content

    # 1. Fix function component props: ({ isVisible }) => ({ isVisible }: { isVisible: boolean })
    # For export default function XXX({ isVisible }) {
    content = re.sub(
        r'(export default function \w+\(\{ isVisible )\}(\))',
        r'\1}: { isVisible: boolean })',
        content
    )

    # 2. Fix arrow component props: function XXX({ xxx, yyy }) to function XXX({ xxx, yyy }: { xxx: any; yyy: any })
    # This is for memo(function Name({ ... })) patterns - skip these as they need manual handling

    # 3. Fix column render/getValue/getNumericValue callbacks: (item) => to (item: any) =>
    # Match (item) => in column definitions
    content = re.sub(r'\(item\) =>', '(item: any) =>', content)

    # 4. Fix (isOpen, buttonElement) => in onOpenChange
    content = re.sub(
        r'onOpenChange=\{\(isOpen, buttonElement\) =>',
        'onOpenChange={(isOpen: any, buttonElement: any) =>',
        content
    )

    # 5. Fix catch (err) to catch (err: any) - but only when err is used after
    content = re.sub(r'} catch \(err\) \{', '} catch (err: any) {', content)

    # 6. Fix useState(null) to useState<any>(null)
    content = re.sub(r'useState\(null\)', 'useState<any>(null)', content)

    # 7. Fix useState([]) to useState<any[]>([]) - only when not already typed
    content = re.sub(r'useState\(\[\]\)', 'useState<any[]>([])', content)

    # 8. Fix helper functions with untyped params like getHosts = (ingress) =>
    # Match const/let/function patterns with simple single-param callbacks
    # e.g., const getHosts = (ingress) =>
    content = re.sub(
        r'(const \w+ = \()(\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any) =>' if m.group(2) not in ('e', 'prev', 'k', 'v', 'a', 'b', 'ns', 'acc') and not m.group(2)[0].isupper() else m.group(0),
        content
    )

    # 9. Fix (val) => in onChange callbacks for SearchSelect
    content = re.sub(r'onChange=\{\(val\) =>', 'onChange={(val: any) =>', content)

    # 10. Fix getOptionValue={(x) => x.value} patterns
    content = re.sub(r'getOptionValue=\{\((\w+)\) =>', r'getOptionValue={(\1: any) =>', content)
    content = re.sub(r'getOptionLabel=\{\((\w+)\) =>', r'getOptionLabel={(\1: any) =>', content)

    # 11. Fix .map(r => patterns in data mapping
    content = re.sub(r'\.map\(r =>', '.map((r: any) =>', content)
    content = re.sub(r'\.map\(p =>', '.map((p: any) =>', content)
    content = re.sub(r'\.map\(e =>', '.map((e: any) =>', content)
    content = re.sub(r'\.map\(c =>', '.map((c: any) =>', content)
    content = re.sub(r'\.map\(s =>', '.map((s: any) =>', content)
    content = re.sub(r'\.map\(t =>', '.map((t: any) =>', content)
    content = re.sub(r'\.map\(d =>', '.map((d: any) =>', content)
    content = re.sub(r'\.map\(n =>', '.map((n: any) =>', content)
    content = re.sub(r'\.map\(a =>', '.map((a: any) =>', content)
    content = re.sub(r'\.map\(w =>', '.map((w: any) =>', content)
    content = re.sub(r'\.map\(h =>', '.map((h: any) =>', content)
    content = re.sub(r'\.map\(l =>', '.map((l: any) =>', content)
    content = re.sub(r'\.map\(i =>', '.map((i: any) =>', content)
    content = re.sub(r'\.map\(v =>', '.map((v: any) =>', content)
    content = re.sub(r'\.map\(b =>', '.map((b: any) =>', content)

    # 12. Fix .find(c => patterns
    content = re.sub(r'\.find\(c =>', '.find((c: any) =>', content)
    content = re.sub(r'\.find\(r =>', '.find((r: any) =>', content)
    content = re.sub(r'\.find\(a =>', '.find((a: any) =>', content)
    content = re.sub(r'\.find\(s =>', '.find((s: any) =>', content)
    content = re.sub(r'\.find\(p =>', '.find((p: any) =>', content)
    content = re.sub(r'\.find\(lb =>', '.find((lb: any) =>', content)
    content = re.sub(r'\.find\(h =>', '.find((h: any) =>', content)
    content = re.sub(r'\.find\(e =>', '.find((e: any) =>', content)
    content = re.sub(r'\.find\(m =>', '.find((m: any) =>', content)
    content = re.sub(r'\.find\(n =>', '.find((n: any) =>', content)

    # 13. Fix .filter(x => patterns
    content = re.sub(r'\.filter\(r =>', '.filter((r: any) =>', content)
    content = re.sub(r'\.filter\(p =>', '.filter((p: any) =>', content)
    content = re.sub(r'\.filter\(c =>', '.filter((c: any) =>', content)
    content = re.sub(r'\.filter\(s =>', '.filter((s: any) =>', content)
    content = re.sub(r'\.filter\(t =>', '.filter((t: any) =>', content)
    content = re.sub(r'\.filter\(h =>', '.filter((h: any) =>', content)
    content = re.sub(r'\.filter\(e =>', '.filter((e: any) =>', content)
    content = re.sub(r'\.filter\(l =>', '.filter((l: any) =>', content)
    content = re.sub(r'\.filter\(n =>', '.filter((n: any) =>', content)
    content = re.sub(r'\.filter\(v =>', '.filter((v: any) =>', content)

    # 14. Fix .some(x => patterns
    content = re.sub(r'\.some\(lb =>', '.some((lb: any) =>', content)
    content = re.sub(r'\.some\(c =>', '.some((c: any) =>', content)
    content = re.sub(r'\.some\(s =>', '.some((s: any) =>', content)
    content = re.sub(r'\.some\(a =>', '.some((a: any) =>', content)
    content = re.sub(r'\.some\(e =>', '.some((e: any) =>', content)
    content = re.sub(r'\.some\(p =>', '.some((p: any) =>', content)

    # 15. Fix .forEach(x => patterns
    content = re.sub(r'\.forEach\(e =>', '.forEach((e: any) =>', content)
    content = re.sub(r'\.forEach\(p =>', '.forEach((p: any) =>', content)
    content = re.sub(r'\.forEach\(c =>', '.forEach((c: any) =>', content)
    content = re.sub(r'\.forEach\(r =>', '.forEach((r: any) =>', content)
    content = re.sub(r'\.forEach\(pair =>', '.forEach((pair: any) =>', content)

    # 16. Fix .reduce((acc, x) => patterns
    content = re.sub(r'\.reduce\(\(acc, (\w+)\) =>', r'.reduce((acc: any, \1: any) =>', content)

    # 17. Fix two-param helper functions: const fn = (x, y) =>
    content = re.sub(
        r'(const \w+ = async \()(\w+), (\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any, {m.group(3)}: any) =>' if m.group(2) not in ('e', 'prev') else m.group(0),
        content
    )

    # 18. Fix useCallback(async (xxx) => patterns
    content = re.sub(
        r'(useCallback\(async \()(\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any) =>' if m.group(2) not in ('e', 'prev') and not m.group(2).startswith('_') else m.group(0),
        content
    )
    content = re.sub(
        r'(useCallback\(\()(\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any) =>' if m.group(2) not in ('e', 'prev') and not m.group(2).startswith('_') else m.group(0),
        content
    )

    # 19. Fix useCallback((x, y) =>
    content = re.sub(
        r'(useCallback\(\()(\w+), (\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any, {m.group(3)}: any) =>' if m.group(2) not in ('e', 'prev') else m.group(0),
        content
    )
    content = re.sub(
        r'(useCallback\(async \()(\w+), (\w+)\) =>',
        lambda m: f'{m.group(1)}{m.group(2)}: any, {m.group(3)}: any) =>' if m.group(2) not in ('e', 'prev') else m.group(0),
        content
    )

    # Don't double-annotate: fix (item: any: any) back to (item: any)
    content = re.sub(r': any: any', ': any', content)

    # Don't double-annotate useState
    content = re.sub(r'useState<any><any>', 'useState<any>', content)
    content = re.sub(r'useState<any\[\]><any\[\]>', 'useState<any[]>', content)

    if content != original:
        with open(filepath, 'w') as f:
            f.write(content)
        return True
    return False

# Get list of files from command line or default to all feature files
if len(sys.argv) > 1:
    files = sys.argv[1:]
else:
    # Find all .tsx files in features directory
    features_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'src', 'features')
    files = []
    for root, dirs, filenames in os.walk(features_dir):
        for f in filenames:
            if f.endswith('.tsx'):
                files.append(os.path.join(root, f))

changed = 0
for filepath in files:
    if fix_file(filepath):
        print(f"Fixed: {os.path.basename(filepath)}")
        changed += 1

print(f"\nTotal files modified: {changed}")
