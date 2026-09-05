-- books-publish-collection.applescript
--
-- Follow-up script that runs AFTER the base `open -a Books path.pdf` has
-- imported a PDF into the Books library. Its only job is to move that book
-- into the named collection (or create the collection if missing).
--
-- Usage: osascript books-publish-collection.applescript PATH COLLECTION
--
-- Requires the user to have granted Automation permission for the shell
-- host to control Books. First-time invocations pop a dialog; if the user
-- denies, this script fails and the parent process just records a
-- "collection move deferred" note on the artifact — the PDF still landed
-- in the base Books library either way.

on run argv
    if (count of argv) < 2 then
        error "usage: books-publish-collection.applescript PATH COLLECTION"
    end if
    set inputPath to item 1 of argv
    set collectionName to item 2 of argv

    -- Give Books a beat to finish ingesting the file that `open -a` handed it.
    delay 1

    -- Match the freshly-imported book by filename stem. If we can't find it,
    -- we bail rather than pick a random one.
    set targetName to my baseNameNoExt(inputPath)

    tell application "Books"
        set matchingBooks to books whose name is targetName
        if (count of matchingBooks) is 0 then
            error "no book matched name: " & targetName
        end if
        set added to first item of matchingBooks

        set targetCollection to my findOrMakeCollection(collectionName)
        if targetCollection is missing value then
            error "could not find or create collection: " & collectionName
        end if
        move added to targetCollection
    end tell
end run

on findOrMakeCollection(nm)
    tell application "Books"
        try
            return collection named nm
        end try
        try
            return first collection whose name is nm
        end try
        try
            return make new collection with properties {name:nm}
        end try
        return missing value
    end tell
end findOrMakeCollection

-- baseNameNoExt("/a/b/foo.pdf") -> "foo"
on baseNameNoExt(p)
    set slash to offset of "/" in (reverse of characters of p) as string
    set fname to text ((length of p) - slash + 2) thru -1 of p
    set dot to offset of "." in (reverse of characters of fname) as string
    if dot is 0 then return fname
    return text 1 thru ((length of fname) - dot) of fname
end baseNameNoExt
