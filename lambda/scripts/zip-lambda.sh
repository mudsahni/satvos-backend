  cd lambda
  rm -rf package lambda.zip
  pip install -r requirements.txt -t package/
  cp -r ses_invoice_processor package/
  cd package && zip -r ../lambda.zip . && cd ..
