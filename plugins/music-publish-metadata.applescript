-- music-publish-metadata.applescript
--
-- Follow-up script that runs AFTER `open -a Music path.mp3` has imported an
-- audio file. Its job is to set genre and optionally add to a playlist.
--
-- Usage: osascript music-publish-metadata.applescript PATH [GENRE] [PLAYLIST]
--
-- Automation permission required for Music.app. Failure here is not fatal —
-- the parent process records a "metadata deferred" note on the artifact
-- and the audio is still in the base library.

on run argv
    if (count of argv) < 1 then
        error "usage: music-publish-metadata.applescript PATH [GENRE] [PLAYLIST]"
    end if
    set inputPath to item 1 of argv
    set genreName to ""
    set playlistName to ""
    if (count of argv) >= 2 then set genreName to item 2 of argv
    if (count of argv) >= 3 then set playlistName to item 3 of argv

    delay 1 -- let Music finish ingesting

    set targetName to my baseNameNoExt(inputPath)

    tell application "Music"
        set matching to (tracks of library playlist 1 whose name is targetName)
        if (count of matching) is 0 then
            error "no track matched name: " & targetName
        end if
        set added to first item of matching

        if genreName is not "" then
            try
                set genre of added to genreName
            end try
        end if

        if playlistName is not "" then
            set targetPlaylist to my findOrMakePlaylist(playlistName)
            if targetPlaylist is not missing value then
                try
                    duplicate added to targetPlaylist
                end try
            end if
        end if
    end tell
end run

on findOrMakePlaylist(nm)
    tell application "Music"
        try
            return user playlist nm
        end try
        try
            return first user playlist whose name is nm
        end try
        try
            return make new user playlist with properties {name:nm}
        end try
        return missing value
    end tell
end findOrMakePlaylist

on baseNameNoExt(p)
    set slash to offset of "/" in (reverse of characters of p) as string
    set fname to text ((length of p) - slash + 2) thru -1 of p
    set dot to offset of "." in (reverse of characters of fname) as string
    if dot is 0 then return fname
    return text 1 thru ((length of fname) - dot) of fname
end baseNameNoExt
