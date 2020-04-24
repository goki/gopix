# GoPix

Go picture management app, because Mac Photos bricked my entire photo library upon updating to Catalina.

# Goals

* simple, entirely file-based design -- no databases.   just write to exif meta data on images themselves.  so everything is future-proof and no lockin.  (does have a .json file cache, but it can be fully regenerated from exif..)

* sync with e.g., google drive using the drive sync thing.

* use gimp for photo retouching, or preview

# File Structure

* Main dir: `~/Pix`
* All images live in one common directory: `~/Pix/All`
* Folders for specific albums have symbolic links to original pics in `../All/img.jpg`
* DND / Copy/Paste operations create these symlinks, except if going into the trash -- or all

# TODO

* Need a busy / progress bar for long-running ops.

* Duplicates detector -- move dupes to trash

* Finish ImgView with auto-size and zoom keyboard commands (+ / -)

