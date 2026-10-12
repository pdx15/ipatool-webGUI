# 🖥️ ipatool GUI Guide for Windows Users

The graphical user interface (GUI) for **ipatool** enables any user without technical or command-line skills to effortlessly search, license, and download iOS/iPadOS `.ipa` packages directly from the Apple App Store on Windows.

---

## 🚀 1-Click Launch

1. **Extract the archive** to any folder on your computer.
2. **Double-click:**
   - **`ipatool-gui.bat`** (or **`ipatool.exe`**).
3. The modern GUI opens automatically in your browser or desktop application window.

---

## 🌟 Key Features

1. **Apple ID Authentication with 2FA Support:**
   - Simple visual login dialog.
   - Interactive modal prompt for 6-digit two-factor verification codes.
   - Safe and direct communication with Apple servers over SRP-6a / GSA protocol.

2. **App Store Search & Discovery:**
   - Search by app name or paste an App Store URL.
   - Filter by iPhone, iPad, or Apple TV.
   - View app icons, prices, versions, and bundle IDs.
   - 1-click Download, 1-click License acquisition, and Version history.

3. **Direct Download:**
   - Download by Bundle Identifier or numeric App ID.
   - Optional version ID targeting.
   - Automatic free license acquisition (`--purchase`).

4. **Version History & Archive Downloader:**
   - Look up all historic version IDs from Apple servers.
   - Download any previously released build with one click.

5. **Download Manager & Windows Explorer Integration:**
   - Live download percentage, speed, and status.
   - "Show in Explorer" button opens the downloaded `.ipa` location immediately.

6. **Install `.IPA` on a connected iOS device:**
   - Automatically detects connected iPhone/iPad devices with `libimobiledevice`.
   - Upload (or reuse) a downloaded `.IPA` and install it with `ideviceinstaller`.
   - Requires `ideviceinstaller` (macOS: `brew install libimobiledevice`, Windows: install a libimobiledevice / iDevice Suite build and put its `bin` on PATH, or extract the build and copy its `.exe`/`.dll` files into the `tools\` folder next to `ipatool.exe` — the GUI detects them automatically). You can also set `IPATOOL_IDEVICEINSTALLER`, `IPATOOL_IDEVICE_ID` and `IPATOOL_IDEVICEDEVICEINFO` to the exact paths.


---

## 📲 Installing Downloaded `.IPA` files on iOS

- **Built-in "Install to device" tab (fastest):** open the **Install to device** tab, click Refresh, choose the connected device, pick a downloaded `.ipa`, and press **Install on device**. This uses `ideviceinstaller` from `libimobiledevice`; if it is missing, install it as described in the feature list above.
- **Sideloadly (Recommended for Windows):** Drag & drop the `.ipa` into Sideloadly, connect iPhone via USB, click Start.
- **AltStore:** Sideload wirelessly over local Wi-Fi.
- **TrollStore:** Permanent install for supported iOS versions without 7-day expiration.
