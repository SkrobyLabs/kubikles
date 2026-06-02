export type SearchHighlightToken = {
    text: string;
    kind: 'plain' | 'regex' | 'invalidRegex';
};

const regexTokenPattern = /^\/(.+?)\/([gimsuy]*)?(?=\s|,|$)/;
const fieldRegexTokenPattern = /^(\w+:)(\/(.+?)\/([gimsuy]*)?)(?=\s|,|$)/;

const isValidRegex = (pattern: string, flags = ''): boolean => {
    try {
        new RegExp(pattern, flags);
        return true;
    } catch {
        return false;
    }
};

const pushToken = (tokens: SearchHighlightToken[], text: string, kind: SearchHighlightToken['kind']) => {
    if (!text) return;
    const previous = tokens[tokens.length - 1];
    if (previous?.kind === kind) {
        previous.text += text;
        return;
    }
    tokens.push({ text, kind });
};

export const tokenizeSearchHighlights = (query: string): SearchHighlightToken[] => {
    if (!query) return [];

    const tokens: SearchHighlightToken[] = [];
    let plainBuffer = '';
    let index = 0;

    while (index < query.length) {
        const remaining = query.slice(index);
        const fieldRegexMatch = remaining.match(fieldRegexTokenPattern);

        if (fieldRegexMatch) {
            pushToken(tokens, plainBuffer, 'plain');
            plainBuffer = '';
            pushToken(tokens, fieldRegexMatch[1], 'plain');
            pushToken(
                tokens,
                fieldRegexMatch[2],
                isValidRegex(fieldRegexMatch[3], fieldRegexMatch[4] || '') ? 'regex' : 'invalidRegex'
            );
            index += fieldRegexMatch[0].length;
            continue;
        }

        const regexMatch = remaining.match(regexTokenPattern);
        if (regexMatch) {
            pushToken(tokens, plainBuffer, 'plain');
            plainBuffer = '';
            pushToken(
                tokens,
                regexMatch[0],
                isValidRegex(regexMatch[1], regexMatch[2] || '') ? 'regex' : 'invalidRegex'
            );
            index += regexMatch[0].length;
            continue;
        }

        plainBuffer += query[index];
        index += 1;
    }

    pushToken(tokens, plainBuffer, 'plain');
    return tokens;
};
