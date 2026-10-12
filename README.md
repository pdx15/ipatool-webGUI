[![GitHub Release](https://img.shields.io/github/release/pdx15/ipatool-webGUI)](https://github.com/pdx15/ipatool-webGUI/releases)
[![Github All Releases](https://img.shields.io/github/downloads/pdx15/ipatool-webGUI/total.svg)](https://github.com/pdx15/ipatool-webGUI/releases)
[![ViewCount](https://views.whatilearened.today/views/github/pdx15/ipatool-webGUI.svg)](https://github.com/pdx15/ipatool-webGUI)
[![Donations Page](https://github.com/andry81-cache/gh-content-static-cache/raw/master/common/badges/donate/donate.svg)](https://github.com/pdx15/ipatool-webGUI#%D0%B4%D0%BE%D0%BD%D0%B0%D1%82donate)
# ipatool webGUI
Download for macOS/windows - https://github.com/pdx15/ipatool-webGUI/releases

DEMO MODE [pdx15.github.io](https://pdx15.github.io/ipatool-webGUI/)

A web-based GUI tool that searches, downloads, and installs App Store app
packages (`.ipa` files) for iOS, iPadOS, tvOS, watchOS, visionOS, and macOS.

![ipatool GUI preview](resources/social-preview.png)

This is an unofficial community project. It is not affiliated with Apple or the
upstream `ipatool` maintainers.

## Features

- **App search** across the official App Store **and** a bundled catalog of apps
  that were removed from the App Store but can still be downloaded by their App
  ID (`Apps_ID_List.txt`, compiled into the binary). The two result groups are
  shown separately, with the removed apps listed first (`/api/search/all`
  answers with both groups in that order, best matches on top).
- **One-click download** of encrypted `.ipa` packages tied to your Apple ID.
- **Mass check & download** from a `.txt` list (`Name: AppID` format). Apps your
  account never owned are filtered out, and the remaining list can be saved back
  to a text file.
- **Version history** for downloading older builds of an app.
- **Install to device** through `libimobiledevice` (`ideviceinstaller`).
- Built-in **Apple ID login** with 2FA support and an alternative macOS login
  path (`Sign In Apple ID SKIP`).
- The GUI automatically creates and reuses ipatool's local keychain passphrase,
  so file-backed storage does not prompt in the terminal. The passphrase stays
  in the per-user ipatool state directory and is never sent to Apple or the
  browser; its at-rest protection relies on the operating system's file
  permissions. A file-backed session encrypted earlier with a different,
  manually entered passphrase may need one new Apple ID login.
- Russian and English interface, dark and light themes.

## Install

Download the release archive, extract it, and grant execute permissions:

```shell
chmod +x ipatool-webGUI-arm64-macOS
xattr -d com.apple.quarantine ipatool-webGUI-arm64-macOS
```

Run the GUI:

```shell
./ipatool-webGUI-arm64-macOS gui
```

For the IPA installer to work, you also need `libimobiledevice`:

```shell
brew install libimobiledevice
```

If you want to install it as the `ipatool` command:

```shell
sudo install -m 0755 ipatool-webGUI-arm64-macOS /usr/local/bin/ipatool
ipatool gui
ipatool --version
```

### Windows

A Windows build ships as `ipatool.exe`. Double-click `ipatool-gui.bat` (or run
`ipatool.exe gui`) and the interface opens in your default browser. See
[WINDOWS_GUI_GUIDE.md](WINDOWS_GUI_GUIDE.md) (English) or
[ИНСТРУКЦИЯ_GUI.md](ИНСТРУКЦИЯ_GUI.md) (Russian) for a detailed walkthrough.

## Requirements and limitations

- App Store authentication in this build requires macOS with cgo enabled.
- Release binaries are provided for Apple Silicon and Intel Macs.
- The App Store protocol is private and can change without notice.
- Downloaded App Store packages remain encrypted and are tied to the Apple ID
  that acquired them.
- Installing `.ipa` from the GUI "Install to device" tab requires
  `libimobiledevice` (`ideviceinstaller`, `idevice_id`, `idevicedeviceinfo`) on
  the machine that has the connected iOS device.
- You are responsible for following Apple's terms and applicable law.

## Credits and license

Based on [`majd/ipatool`](https://github.com/majd/ipatool) and distributed under the [MIT License](LICENSE). The original copyright and
license notice are preserved.

## Донат/Donate

Если вы хотите оставить чаевые, то можно это сделать по ссылке ниже или через QR-код:

[![Поддержать через CloudTips](resources/qrCode.png)](https://pay.cloudtips.ru/p/1569852d)

**Оставить чаевые через CloudTips:** https://pay.cloudtips.ru/p/1569852d

| Crypto Currency | Network | Wallet ID |
|--------------|------|----------------|
| **USDT**     | TRC20 | `TGm951N5TpuBKayeAP6vL8wAdB9vEvdMyo` |
| **USDT**     | ERC20 | `0xd938130bce0C9A6a8F83dDe4fEAF6A86847c148B` |
| **BTC**      |    | `c1qhg00wd03fnhdpwhg8fa4a3tpshhmsf0uslw8nf` |
