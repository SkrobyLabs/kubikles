# Linux Support

Kubikles runs on Linux but requires GTK3 and WebKit2GTK runtime libraries.

## Runtime Dependencies

Linux binaries are dynamically linked to GTK3 and WebKit2GTK. Install them for your distribution:

```bash
# Arch / CachyOS / Manjaro
sudo pacman -S gtk3 webkit2gtk

# Debian / Ubuntu (< 24.04)
sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37

# Ubuntu 24.04+
sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0

# Fedora
sudo dnf install gtk3 webkit2gtk4.1

# openSUSE
sudo zypper install gtk3 webkit2gtk3
```

## Development Setup

For development, you also need the development headers:

```bash
# Arch / CachyOS / Manjaro
sudo pacman -S gtk3 webkit2gtk base-devel go nodejs npm

# Debian / Ubuntu (< 24.04)
sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.0-dev golang nodejs npm

# Ubuntu 24.04+
sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev golang nodejs npm
# Note: Build with -tags webkit2_41

# Fedora
sudo dnf install gtk3-devel webkit2gtk4.1-devel gcc-c++ golang nodejs npm
```

Or simply run `make setup` which handles this automatically.

## AppImage

The AppImage format creates a portable single-file executable:

```bash
make build-appimage
./build/Kubikles-x86_64.AppImage
```

Note: AppImage still requires GTK3/WebKit2GTK on the target system (these cannot be fully statically linked due to their deep system integration).

## Why Not Static Linking?

GTK and WebKit cannot be practically statically linked - they have deep dependencies on system libraries, fonts, themes, and D-Bus. This is a limitation of all GTK-based applications, not specific to Kubikles.

For truly portable distribution, consider:
- **AppImage** - bundles the app, users install system deps
- **Flatpak** - sandboxed with runtime dependencies
- **Snap** - Ubuntu-native packaging
- **Docker** - containerized with X11 forwarding
