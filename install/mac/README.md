# Mac Install

The Makefile has targets for installing the app:

* `app-install` copies the compiled executable from the `go install` version, in ~/go/bin/gopix, then installs the GoPix.app in `/Applications`

* `dev-install` installs an .app with a script that runs the `go install` version directly, so any time you do go install it runs that updated version.

* `app-dmg` makes a .dmg with the GoPix.app -- copies the compiled executable per app-install first.

