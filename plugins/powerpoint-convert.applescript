-- powerpoint-convert.applescript
-- Convert a PowerPoint presentation to PDF via Microsoft PowerPoint.
--
-- Usage: osascript powerpoint-convert.applescript INPUT OUTPUT
--
-- Modern PowerPoint's `open` command doesn't return a presentation reference
-- in all versions; grab `active presentation` after the open instead. Also
-- avoids referencing thePres before it's set (which produced -2753 errors).

on run argv
    if (count of argv) < 2 then
        error "usage: powerpoint-convert.applescript INPUT OUTPUT"
    end if
    set inputPath to item 1 of argv
    set outputPath to item 2 of argv

    set inputPosix to POSIX file inputPath
    set outputPosix to POSIX file outputPath

    tell application "Microsoft PowerPoint"
        activate
        open inputPosix
        set thePres to active presentation
        try
            save thePres in outputPosix as save as PDF
        on error errMsg number errNum
            try
                close thePres saving no
            end try
            error errMsg number errNum
        end try
        close thePres saving no
    end tell
end run
