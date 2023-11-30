class AddHeader:
    def __init__(self):
        self.num = 0

    def response(self, flow):
        self.num = self.num + 1
        print("got response num", self.num)
        flow.response.headers["count"] = str(self.num)
        