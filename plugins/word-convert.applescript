-- word-convert.applescript
-- Convert a Word document to PDF via Microsoft Word.
--
-- Usage: osascript word-convert.applescript INPUT OUTPUT FORMAT
--   FORMAT: pdf
--
-- Note: we only handle PDF here. Text extraction via Word AppleScript is
-- unreliable — `save as ... file format format Unicode text` throws -1708
-- on modern Word for Mac, and reading `content of text object` hangs on
-- large documents. The dispatcher instead routes docx -> txt through
-- pandoc-text / libreoffice-headless, which are both fast and correct.

on run argv
    if (count of argv) < 3 then
        error "usage: word-convert.applescript INPUT OUTPUT FORMAT"
    end if
    set inputPath to item 1 of argv
    set outputPath to item 2 of argv
    set outFmt to item 3 of argv

    if outFmt is not "pdf" then
        error "microsoft-word-macos only handles pdf output; got: " & outFmt
    end if

    set inputPosix to POSIX file inputPath
    set outputPosix to POSIX file outputPath

    tell application "Microsoft Word"
        activate
        set theDoc to open file name inputPosix
        try
            save as theDoc file name outputPosix file format format PDF
        on error errMsg number errNum
            close theDoc saving no
            error errMsg number errNum
        end try
        close theDoc saving no
    end tell
end run
