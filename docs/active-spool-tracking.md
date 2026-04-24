# Active Spool Tracking

Active spool tracking lets you associate a physical filament spool with a Spoolman spool ID by tapping an NFC sticker or scanning a QR code. When a print finishes, bambu-middleman uses the active spool to record filament consumption in Spoolman.

> **AMS not supported.** AMS multi-spool setups are not currently supported. Only the external spool slot (vt_tray) is handled. AMS tray changes are ignored. If you have an AMS and would like to help develop support, see [Compatibility & help wanted](../README.md#compatibility--help-wanted).

## Enabling

Set `WEBUI_ADDR` to start the built-in HTTP server alongside the observer:

```
WEBUI_ADDR=:8080
```

If unset, no HTTP server starts and active spool tracking is unavailable.

Also set `WEBUI_BASE_URL` to the externally reachable address of the server -- this is baked into generated QR codes:

```
WEBUI_BASE_URL=http://192.168.1.10:8080
```

Active spool tracking requires the audit database (`AUDIT_DB_PATH`). The observer will refuse to start if `WEBUI_ADDR` is set without `AUDIT_DB_PATH`.

## Setting up a spool

1. Find your spool's ID in Spoolman (visible in the UI or API).
2. Print a label: open `http://<server>/spool/<id>/label` in a browser and print the page. The label contains a QR code and the filament details (manufacturer, name, material). Add `?orientation=horizontal` for a wide label instead of the default tall one.
3. Alternatively, program an NFC sticker with the URL `http://<server>/spool/<id>/activate`.

> A raw QR code PNG is also available at `/spool/<id>/qr` if you need to embed it elsewhere.

## Using it

Tap the NFC sticker or scan the QR code. A confirmation page opens in the browser -- tap **Activate** to set the spool as active. This prevents accidental switches from unintentional taps.

To clear the active spool without loading new filament, open `http://<server>/spool/clear` in the browser and confirm.

## When the active spool sets and clears

```
[you load spool #42 into printer]
-> open /spool/42/activate, tap Activate
-> active spool: #42

[print starts and finishes]
-> spool #42 charged in Spoolman
-> active spool: #42  (still set -- spool is still loaded)

[another print finishes]
-> spool #42 charged again
-> active spool: #42

[you unload spool #42 and load spool #7]
-> printer emits filament load event
-> active spool: cleared automatically

[open /spool/7/activate, tap Activate]
-> active spool: #7
```

## Automatic clearing

bambu-middleman listens to the printer's MQTT feed. When a filament load is detected -- the moment filament passes through the runout switch -- the active spool is cleared automatically. Tap the new spool's NFC or QR code after loading to set the next active spool.

This only applies to the external spool slot (vt_tray). AMS tray changes are ignored.

You can also clear the active spool manually at any time by opening `http://<server>/spool/clear` in a browser and confirming.
