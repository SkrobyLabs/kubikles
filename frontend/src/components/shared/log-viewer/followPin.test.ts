import { describe, it, expect } from 'vitest';
import { createFollowPin, PIN_SLACK_PX, USER_INPUT_WINDOW_MS, followKeyDirection } from './followPin';

describe('createFollowPin', () => {
    const make = () => {
        let t = 1000;
        const pin = createFollowPin(() => t);
        return { pin, advance: (ms: number) => { t += ms; } };
    };

    it('starts pinned and ignores scrolls that are not user driven', () => {
        const { pin } = make();
        expect(pin.isPinned()).toBe(true);
        // Content grew faster than the list scrolled: a large gap with no user input.
        pin.noteScroll(5000);
        pin.noteAtBottom(false);
        expect(pin.isPinned()).toBe(true);
    });

    it('releases on wheel up and restores when the user scrolls back to the end', () => {
        const { pin, advance } = make();
        pin.noteWheel(-40);
        expect(pin.isPinned()).toBe(false);
        pin.noteScroll(900);
        expect(pin.isPinned()).toBe(false);
        advance(2000);
        pin.noteWheel(60);
        pin.noteScroll(PIN_SLACK_PX);
        expect(pin.isPinned()).toBe(true);
    });

    it('stays released when content grows after the user scrolled away', () => {
        const { pin, advance } = make();
        pin.noteWheel(-40);
        advance(USER_INPUT_WINDOW_MS + 1);
        pin.noteScroll(0);
        expect(pin.isPinned()).toBe(false);
    });

    it('follows a scrollbar drag for as long as the pointer is held', () => {
        const { pin, advance } = make();
        pin.setHeld(true);
        advance(5000);
        pin.noteScroll(700);
        expect(pin.isPinned()).toBe(false);
        pin.noteScroll(10);
        expect(pin.isPinned()).toBe(true);
        pin.setHeld(false);
        advance(USER_INPUT_WINDOW_MS + 1);
        pin.noteScroll(700);
        expect(pin.isPinned()).toBe(true);
    });

    it('does not release on a transient gap right after scrolling down to the end', () => {
        const { pin } = make();
        pin.noteWheel(-40);
        pin.noteWheel(5000);
        pin.noteScroll(0);
        expect(pin.isPinned()).toBe(true);
        // The list re-measures rows and reports a gap within the input window.
        pin.noteScroll(900);
        expect(pin.isPinned()).toBe(true);
        pin.noteUserInput('down');
        pin.noteScroll(900);
        expect(pin.isPinned()).toBe(true);
    });

    it('treats keyboard or touch input as user scrolling', () => {
        const { pin } = make();
        pin.noteUserInput();
        pin.noteScroll(PIN_SLACK_PX + 1);
        expect(pin.isPinned()).toBe(false);
    });

    it('restores on the list at-bottom signal and on an explicit pin', () => {
        const { pin } = make();
        pin.unpin();
        pin.noteAtBottom(true);
        expect(pin.isPinned()).toBe(true);
        pin.unpin();
        pin.pin();
        expect(pin.isPinned()).toBe(true);
    });

    it('releases on one upward key press even though it scrolls less than the slack', () => {
        const { pin } = make();
        pin.noteUserInput('up');
        expect(pin.isPinned()).toBe(false);
        // One arrow press scrolls about 40px, inside the slack: it must not pin again.
        pin.noteScroll(40);
        expect(pin.isPinned()).toBe(false);
        // Moving back towards the end does.
        pin.noteUserInput('down');
        pin.noteScroll(0);
        expect(pin.isPinned()).toBe(true);
    });

    it('does not pin again on a short upward wheel tick inside the slack', () => {
        const { pin } = make();
        pin.noteWheel(-10);
        pin.noteScroll(10);
        expect(pin.isPinned()).toBe(false);
    });
});

describe('followKeyDirection', () => {
    it('maps scrolling keys and ignores the rest', () => {
        expect(followKeyDirection('ArrowUp', false)).toBe('up');
        expect(followKeyDirection('PageUp', false)).toBe('up');
        expect(followKeyDirection('Home', false)).toBe('up');
        expect(followKeyDirection(' ', true)).toBe('up');
        expect(followKeyDirection('ArrowDown', false)).toBe('down');
        expect(followKeyDirection('End', false)).toBe('down');
        expect(followKeyDirection(' ', false)).toBe('down');
        expect(followKeyDirection('c', false)).toBeNull();
        expect(followKeyDirection('Shift', true)).toBeNull();
        expect(followKeyDirection('Tab', false)).toBeNull();
    });
});
