# Kubikles Style Guide

## Color Palette

### Core Colors
These are the foundational colors for the application's dark theme.

| Color Name | Hex Code | Description |
| :--- | :--- | :--- |
| **Background** | `#1e1e1e` | Main application background. |
| **Surface** | `#252526` | Secondary background for panels, sidebars, and headers. |
| **Primary** | `#007acc` | Primary accent color for active states, links, and focus. |
| **Text** | `#cccccc` | Primary text color. |
| **Border** | `#3e3e42` | Border color for separators and inputs. |

### Status Colors
Use these colors to convey state and feedback.

#### Success (Green)
Used for healthy states, successful operations, and "Ready" statuses.
- **Base**: `#4CC38A` (Soft mint-green pop)
- **Dark**: `#3AA876` (Darker variant for hover/active)

#### Error (Red)
Used for error states, failures, destructive actions, and "Terminating" statuses.
- **Base**: `#E5484D` (Muted crimson)
- **Dark**: `#C33A3F` (Darker variant for hover/active)

#### Warning (Orange)
Used for warning states, pending operations, and non-critical issues.
- **Base**: `#F5A623` (Warm, balanced amber)
- **Dark**: `#D98C1C` (Darker variant for hover/active)

#### In-Between (Red-Orange)
Used for transitional states or high-priority warnings.
- **Base**: `#E66B2F` (Burnt orange leaning red)
- **Dark**: `#C75A27` (Darker variant for hover/active)

## Typography
- **Font Family**: Sans-serif (Inter/System default)
- **Base Size**: 14px (text-sm)

## Usage Guidelines
- **Backgrounds**: Use `bg-background` for the main canvas and `bg-surface` for contained areas.
- **Text**: Use `text-text` for primary content. Mute secondary text with `text-gray-400`.
- **Interactive Elements**: Use `hover:bg-white/5` or `hover:bg-surface-hover` for subtle interactions.
- **Status Indicators**: Use the appropriate status color for text or icons (e.g., `text-success`, `text-error`).
