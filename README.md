# GoPix

Go picture management app, because Mac Photos bricked my entire photo library upon updating to Catalina.

# Goals

* simple, entirely file-based design -- no databases.   just write to exif meta data on images themselves.  so everything is future-proof and no lockin.

* sync with e.g., google drive using the drive sync thing.

* use gimp for photo retouching, or preview

# File Structure

* Main dir: `~/pix`
* All images live in one common directory: `~/pix/all`
* Folders for specific albums have symbolic links to original pics in `../all/img.jpg`
* DND / Copy/Paste operations create these symlinks, except if going into the trash -- or all

# TODO

* imggrid actions and filetree dnd to do basic actions.  trash for trash.  all is diff from other folders.

* set symlink for dragging in treeview into folders

