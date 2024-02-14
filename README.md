tool i ~~not so~~ quickly threw together to modify any file(s) in an EVR manifest/package combo.

thank you Exhibitmark for doing the hard work and making [carnation](https://github.com/Exhibitmark/carnation), saved me a lot of headache reversing the manifest format :)



extracting files example:
```
evr_replacefile -mode extract -packageName 48037dc70b0ecab2 -dataDir C:\Games\Oculus\Software\ready-at-dawn-echo-arena\_data\5932408047\rad15\win10 -outputFolder ./output/
```
this will extract and write out every file contained in the package to outputFolder.
the names of the subfolders created in outputFolder are the filetype symbols, the files contained within are named with their respective symbols.

replacing files example:
```
evr_replacefile -mode replace -packageName 48037dc70b0ecab2 -dataDir C:\Games\Oculus\Software\ready-at-dawn-echo-arena\_data\5932408047\rad15\win10 -modifiedFolder C:\Games\Oculus\Software\ready-at-dawn-echo-arena\_data\5932408047\rad15\win10\modified\ -outputFolder ./output/
```
this will read all files in modifiedFolder, expecting the same folder structure as -mode extract outputs.
if a file with the same filetype symbol & filename symbol exists in the manifest, it will edit the manifest & package file to match, and write out the contents of both to outputFolder.
